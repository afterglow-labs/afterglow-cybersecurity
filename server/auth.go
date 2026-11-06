package server

import (
	"crypto/rand"
	"errors"
	"github.com/afterglow-labs/afterglow-cybersecurity/locale"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	jwtKeyFilename   = "jwt.key"
	authPassKey      = "auth.password_hash"
	jwtTTL           = 7 * 24 * time.Hour
	adminSubject     = "Afterglow"
	keyChars         = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	minPasswordRunes = 8
	maxPasswordBytes = 72
)

// Database read failures must never be treated as an uninitialized password.
const errDataSourceUnavailable = "Data source is temporarily unavailable; please try again later"

func validatePassword(pw string) error {
	if utf8.RuneCountInString(pw) < minPasswordRunes {
		return locale.Errorf("Password must contain at least %d characters", minPasswordRunes)
	}
	if len(pw) > maxPasswordBytes {
		return locale.Errorf("Password must not exceed %d bytes", maxPasswordBytes)
	}
	return nil
}

// loadOrCreateJWTKey reads the 32-byte signing key from keyDir/jwt.key. keyDir is
// the project base dir (next to the executable), NOT the browsable workspace root
// (dataDir) — the signing key must never be listable/downloadable via the file
// manager. Legacy installs kept it at dataDir/jwt.key; if present there and not yet
// at the new location, it is migrated (key preserved, so sessions stay valid) and
// the old file removed so it disappears from the workspace. On first run a random
// key is generated and persisted.
func loadOrCreateJWTKey(keyDir, dataDir string) ([]byte, error) {
	path := filepath.Join(keyDir, jwtKeyFilename)
	// one-time migration out of the old in-workspace location.
	if legacy := filepath.Join(dataDir, jwtKeyFilename); legacy != path {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if data, rerr := os.ReadFile(legacy); rerr == nil {
				if len(strings.TrimSpace(string(data))) < 32 {
					return nil, locale.Errorf("legacy JWT key is invalid")
				}
				if werr := createJWTKeyFile(path, data); werr != nil {
					return nil, locale.Errorf("migrate JWT key: %w", werr)
				}
				if rerr := os.Remove(legacy); rerr != nil {
					return nil, locale.Errorf("remove legacy JWT key: %w", rerr)
				}
				log.Printf(locale.Text(locale.ServerDefault(), "[auth] JWT key migrated from %s to %s (outside the browsable workspace)"), legacy, path)
			} else if !os.IsNotExist(rerr) {
				return nil, locale.Errorf("read legacy JWT key: %w", rerr)
			}
		}
	}
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return nil, locale.Errorf("JWT key path is not a regular private file")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, locale.Errorf("inspect JWT key: %w", err)
	}
	if data, err := os.ReadFile(path); err == nil {
		key := []byte(strings.TrimSpace(string(data)))
		if len(key) < 32 {
			return nil, locale.Errorf("stored JWT key is invalid; refusing to rotate encrypted task credentials")
		}
		if err := os.Chmod(path, 0600); err != nil {
			return nil, locale.Errorf("protect JWT key: %w", err)
		}
		return key, nil
	} else if !os.IsNotExist(err) {
		return nil, locale.Errorf("read JWT key: %w", err)
	}
	buf := make([]byte, 32)
	for i := range buf {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(keyChars))))
		if err != nil {
			return nil, locale.Errorf("generate jwt key: %w", err)
		}
		buf[i] = keyChars[n.Int64()]
	}
	err := createJWTKeyFile(path, buf)
	if os.IsExist(err) {
		return loadOrCreateJWTKey(keyDir, dataDir)
	}
	if err != nil {
		return nil, locale.Errorf("write jwt key: %w", err)
	}
	log.Printf(locale.Text(locale.ServerDefault(), "[auth] New JWT key written to %s"), path)
	return buf, nil
}

func createJWTKeyFile(path string, key []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	n, writeErr := file.Write(key)
	if writeErr == nil && n != len(key) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return errors.Join(writeErr, closeErr)
	}
	return nil
}

// adminUsername belongs to this APTEX deployment and defaults to Afterglow.
func adminUsername() string {
	name := strings.TrimSpace(os.Getenv("AFTERGLOW_ADMIN_USERNAME"))
	if name == "" || strings.EqualFold(name, "ARTEX") || len(name) > 64 || strings.ContainsAny(name, "\r\n\t") {
		return adminSubject
	}
	return name
}

// signJWT issues a 7-day administrator token under the configured identity.
func signJWT(key []byte) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   adminUsername(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(jwtTTL)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}).SignedString(key)
}

func parseJWTClaims(tokenStr string, key []byte) (*jwt.RegisteredClaims, bool) {
	claims := &jwt.RegisteredClaims{}
	t, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, locale.Errorf("unexpected signing method")
		}
		return key, nil
	})
	return claims, err == nil && t.Valid && claims.ExpiresAt != nil
}

// verifyJWT accepts only the current administrator identity. Old ARTEX tokens
// are intentionally invalidated with the login rename.
func verifyJWT(tokenStr string, key []byte) bool {
	claims, valid := parseJWTClaims(tokenStr, key)
	return valid && claims.Subject == adminUsername()
}

func adminLoginUsernameAllowed(username string) bool {
	name := strings.TrimSpace(username)
	return !strings.EqualFold(name, "ARTEX") &&
		(strings.EqualFold(name, adminUsername()) || strings.EqualFold(name, adminSubject))
}

// extractToken reads the JWT from Authorization: Bearer header,
// afterglow_token cookie, or ?token= query param (for SSE connections).
func extractToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if c, err := r.Cookie("afterglow_token"); err == nil && c.Value != "" {
		return c.Value
	}
	return r.URL.Query().Get("token")
}

// requireAuth wraps h with JWT validation.
// /api/auth/* and /api/health are exempt.
func (s *Server) requireAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/auth/") || p == "/api/health" {
			h.ServeHTTP(w, r)
			return
		}
		tok := extractToken(r)
		if tok == "" {
			writeErr(w, 401, locale.Text(responseLanguage(w), "Unauthorized"))
			return
		}
		if verifyJWT(tok, s.jwtKey) {
			h.ServeHTTP(w, r)
			return
		}
		// The gateway credential is accepted only in an Authorization header.
		// A cookie or query token must never grant service access.
		headerToken, hasBearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if hasBearer && verifyGatewayJWT(headerToken, s.jwtKey) {
			if !gatewayRouteAllowed(r) {
				writeErr(w, 403, "Gateway token cannot access this route")
				return
			}
			h.ServeHTTP(w, r)
			return
		}
		writeErr(w, 401, locale.Text(responseLanguage(w), "Invalid or expired token"))
	})
}

// GET /api/auth/status — reports whether the admin password has been initialised.
func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	hash, _, err := pg.GetSetting(authPassKey)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	writeJSON(w, 200, map[string]any{"initialized": hash != "", "username": adminUsername()})
}

// POST /api/auth/init — sets the password for the first time; rejected if already set.
func (s *Server) authInit(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	existing, _, err := pg.GetSetting(authPassKey)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if existing != "" {
		writeErr(w, 403, locale.Text(responseLanguage(w), "Password is already set"))
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil || req.Password == "" {
		writeErr(w, 400, locale.Text(responseLanguage(w), "Password cannot be empty"))
		return
	}
	if err := validatePassword(req.Password); err != nil {
		writeError(w, 400, err)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, locale.Text(responseLanguage(w), "Password hashing failed"))
		return
	}
	// The unique constraint protects against concurrent initialization requests.
	inserted, err := pg.InsertSettingIfAbsent(authPassKey, string(hash))
	if err != nil {
		writeErr(w, 500, locale.Text(responseLanguage(w), "Save failed: ")+err.Error())
		return
	}
	if !inserted {
		writeErr(w, 403, "Password is already set")
		return
	}
	tok, err := signJWT(s.jwtKey)
	if err != nil {
		writeErr(w, 500, locale.Text(responseLanguage(w), "Token generation failed"))
		return
	}
	writeJSON(w, 200, map[string]any{"token": tok})
}

// POST /api/auth/change-password — changes the admin password. Requires a valid
// token (this route is under /api/auth/* which requireAuth exempts, so the token
// is validated here) AND the current password.
func (s *Server) authChangePassword(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	if !verifyJWT(extractToken(r), s.jwtKey) {
		writeErr(w, 401, locale.Text(responseLanguage(w), "Unauthorized"))
		return
	}
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, locale.Text(responseLanguage(w), "Invalid request format"))
		return
	}
	if req.NewPassword == "" {
		writeErr(w, 400, locale.Text(responseLanguage(w), "New password cannot be empty"))
		return
	}
	if err := validatePassword(req.NewPassword); err != nil {
		writeError(w, 400, err)
		return
	}
	hash, ok, err := pg.GetSetting(authPassKey)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if !ok || hash == "" {
		writeErr(w, 403, locale.Text(responseLanguage(w), "Password is not initialized; set it first"))
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)); err != nil {
		writeErr(w, 401, locale.Text(responseLanguage(w), "Current password is incorrect"))
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, locale.Text(responseLanguage(w), "Password hashing failed"))
		return
	}
	if err := pg.SetSetting(authPassKey, string(newHash)); err != nil {
		writeErr(w, 500, locale.Text(responseLanguage(w), "Save failed: ")+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/auth/login — validates username/password and returns a JWT.
func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, locale.Text(responseLanguage(w), "Invalid request format"))
		return
	}
	if !adminLoginUsernameAllowed(req.Username) {
		writeErr(w, 401, locale.Text(responseLanguage(w), "Incorrect username or password"))
		return
	}
	hash, ok, err := pg.GetSetting(authPassKey)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if !ok || hash == "" {
		writeErr(w, 403, locale.Text(responseLanguage(w), "Password is not initialized; set it first"))
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		writeErr(w, 401, locale.Text(responseLanguage(w), "Incorrect username or password"))
		return
	}
	tok, err := signJWT(s.jwtKey)
	if err != nil {
		writeErr(w, 500, locale.Text(responseLanguage(w), "Token generation failed"))
		return
	}
	writeJSON(w, 200, map[string]any{"token": tok})
}
