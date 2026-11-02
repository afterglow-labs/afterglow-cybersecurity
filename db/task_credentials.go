package db

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// TaskCredential is a per-task test account (#3): either declared by the user at
// task creation (source=user, e.g. a program-provided alias account) or registered
// by the agent itself and recorded so it is not forgotten (source=agent).
//
// Passwords are encrypted before storage in PostgreSQL. The API returns only
// metadata unless the operator explicitly requests one secret.
type TaskCredential struct {
	ID        int64     `json:"id"`
	TaskID    int64     `json:"task_id"`
	Domain    string    `json:"domain"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	Source    string    `json:"source"` // "user" | "agent"
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TaskCredentialInput validates and normalizes one upsert. Empty username or
// domain is rejected; an empty password is allowed only for source=user (the
// "agent generates and records it later" mode from #3).
type TaskCredentialInput struct {
	Domain   string
	Username string
	Password string
	Source   string
	Note     string
}

var ErrTaskCredentialInvalid = errors.New("credential domain and username are required")

// SetTaskCredentialKey derives a separate AES-256 key from the installation's
// protected signing key. Call once during server startup before any task runs.
func (d *DB) SetTaskCredentialKey(signingKey []byte) error {
	if len(signingKey) < 32 {
		return errors.New("test credential key is too short")
	}
	h := sha256.New()
	_, _ = h.Write([]byte("afterglow/task-credentials/aes-gcm/v1\x00"))
	_, _ = h.Write(signingKey)
	var key [32]byte
	copy(key[:], h.Sum(nil))
	d.credentialMu.Lock()
	d.credentialKey = key
	d.credentialReady = true
	d.credentialMu.Unlock()
	return nil
}

func (d *DB) credentialCipher() (cipher.AEAD, error) {
	d.credentialMu.RLock()
	defer d.credentialMu.RUnlock()
	if !d.credentialReady {
		return nil, errors.New("test credential storage is unavailable")
	}
	block, err := aes.NewCipher(d.credentialKey[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (d *DB) sealCredential(secret string) (string, error) {
	if secret == "" {
		return "", nil
	}
	gcm, err := d.credentialCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(secret), nil)
	return "v1:" + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (d *DB) openCredential(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, "v1:") {
		return "", errors.New("unsupported test credential format")
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, "v1:"))
	if err != nil {
		return "", errors.New("invalid test credential ciphertext")
	}
	gcm, err := d.credentialCipher()
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("invalid test credential ciphertext")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("test credential decryption failed")
	}
	return string(plain), nil
}

func (in TaskCredentialInput) normalize() (TaskCredentialInput, error) {
	in.Domain = strings.TrimSpace(in.Domain)
	in.Username = strings.TrimSpace(in.Username)
	in.Note = strings.TrimSpace(in.Note)
	if in.Source != "agent" {
		in.Source = "user"
	}
	if in.Domain == "" || in.Username == "" || len(in.Domain) > 255 || len(in.Username) > 320 ||
		len(in.Password) > 4096 || len(in.Note) > 1000 || strings.ContainsAny(in.Domain, "/?#@") ||
		strings.IndexFunc(in.Domain, unicode.IsSpace) >= 0 ||
		strings.IndexFunc(in.Username, unicode.IsControl) >= 0 ||
		strings.IndexFunc(in.Note, unicode.IsControl) >= 0 {
		return in, ErrTaskCredentialInvalid
	}
	in.Domain = strings.ToLower(in.Domain)
	if in.Password == "" && in.Source == "agent" {
		return in, errors.New("agent-recorded credentials need the password that was set")
	}
	return in, nil
}

// UpsertTaskCredential inserts or updates the (task, domain, username) entry: an
// agent re-registering the same account overwrites the password in place instead
// of piling up a second row.
func (d *DB) UpsertTaskCredential(taskID int64, in TaskCredentialInput) (TaskCredential, error) {
	if taskID <= 0 {
		return TaskCredential{}, fmt.Errorf("invalid task ID")
	}
	in, err := in.normalize()
	if err != nil {
		return TaskCredential{}, err
	}
	sealed, err := d.sealCredential(in.Password)
	if err != nil {
		return TaskCredential{}, err
	}
	var c TaskCredential
	err = d.QueryRow(`
		INSERT INTO task_credentials (task_id, domain, username, password, source, note)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (task_id, domain, username) DO UPDATE
		SET password = EXCLUDED.password,
		    source   = EXCLUDED.source,
		    note     = CASE WHEN EXCLUDED.note = '' THEN task_credentials.note ELSE EXCLUDED.note END,
		    updated_at = now()
		RETURNING id, task_id, domain, username, password, source, note, created_at, updated_at`,
		taskID, in.Domain, in.Username, sealed, in.Source, in.Note).
		Scan(&c.ID, &c.TaskID, &c.Domain, &c.Username, &c.Password, &c.Source, &c.Note, &c.CreatedAt, &c.UpdatedAt)
	if err == nil {
		c.Password = in.Password
	}
	return c, err
}

// TaskCredentials lists a task's credentials, oldest first (the user-declared
// ones keep their position ahead of agent-discovered ones).
func (d *DB) TaskCredentials(taskID int64) ([]TaskCredential, error) {
	rows, err := d.Query(`
		SELECT id, task_id, domain, username, password, source, note, created_at, updated_at
		FROM task_credentials WHERE task_id = $1 ORDER BY id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskCredential
	for rows.Next() {
		var c TaskCredential
		if err := rows.Scan(&c.ID, &c.TaskID, &c.Domain, &c.Username, &c.Password, &c.Source, &c.Note, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Password, err = d.openCredential(c.Password)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (d *DB) TaskCredentialSecret(taskID, id int64) (string, error) {
	var stored string
	if err := d.QueryRow(`SELECT password FROM task_credentials WHERE id=$1 AND task_id=$2`, id, taskID).Scan(&stored); err != nil {
		return "", err
	}
	return d.openCredential(stored)
}

// HasTaskCredential verifies a reference without decrypting or returning a secret.
func (d *DB) HasTaskCredential(taskID, id int64) (bool, error) {
	if taskID <= 0 || id <= 0 {
		return false, nil
	}
	var exists bool
	err := d.QueryRow(`SELECT EXISTS(SELECT 1 FROM task_credentials WHERE task_id=$1 AND id=$2 AND password <> '')`, taskID, id).Scan(&exists)
	return exists, err
}

func (s *ExplorationStore) HasTaskCredential(taskID, id int64) (bool, error) {
	return s.db.HasTaskCredential(taskID, id)
}

// DeleteTaskCredential removes one entry; taskID scopes the delete so a stale
// request cannot cross tasks.
func (d *DB) DeleteTaskCredential(taskID, id int64) error {
	tag, err := d.Exec(`DELETE FROM task_credentials WHERE id = $1 AND task_id = $2`, id, taskID)
	if err != nil {
		return err
	}
	if n, _ := tag.RowsAffected(); n == 0 {
		return errors.New("credential not found")
	}
	return nil
}

// Thin wrappers on ExplorationStore: the ToolSet already holds the task's
// exploration store, so credential tools need no extra wiring.

func (s *ExplorationStore) UpsertTaskCredential(taskID int64, in TaskCredentialInput) (TaskCredential, error) {
	return s.db.UpsertTaskCredential(taskID, in)
}

func (s *ExplorationStore) TaskCredentials(taskID int64) ([]TaskCredential, error) {
	return s.db.TaskCredentials(taskID)
}
