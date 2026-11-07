package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type AuthSession struct {
	Username  string
	ExpiresAt time.Time
	Revoked   bool
}

// AuthSessionPolicy reads revocation and legacy acceptance in one snapshot.
func (d *DB) AuthSessionPolicy(ctx context.Context, hash string) (*AuthSession, bool, error) {
	var found bool
	var row AuthSession
	var legacyDisabled bool
	err := d.QueryRowContext(ctx, `SELECT s.token_hash IS NOT NULL,
        COALESCE(s.username,''), COALESCE(s.expires_at,to_timestamp(0)),
        COALESCE(s.revoked,false),
        COALESCE((SELECT value='true' FROM settings WHERE key='auth.sessions.legacy_disabled'),false)
        FROM (VALUES(1)) seed(n) LEFT JOIN auth_sessions s ON s.token_hash=$1`, hash).
		Scan(&found, &row.Username, &row.ExpiresAt, &row.Revoked, &legacyDisabled)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, !legacyDisabled, nil
	}
	return &row, !legacyDisabled, nil
}

// RegisterAuthSession never replaces a revocation tombstone.
func (d *DB) RegisterAuthSession(ctx context.Context, hash, username string, expires time.Time) error {
	_, err := d.ExecContext(ctx, `INSERT INTO auth_sessions(token_hash,username,expires_at)
        VALUES($1,$2,$3) ON CONFLICT(token_hash) DO NOTHING`, hash, username, expires)
	return err
}

func (d *DB) RestoreLegacyAuthSession(ctx context.Context, hash, username string, expires time.Time) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var password string
	if err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='auth.password_hash' FOR SHARE`).Scan(&password); err != nil {
		return err
	}
	var disabled bool
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT value='true' FROM settings WHERE key='auth.sessions.legacy_disabled'),false)`).Scan(&disabled); err != nil {
		return err
	}
	if disabled {
		return sql.ErrNoRows
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO auth_sessions(token_hash,username,expires_at) VALUES($1,$2,$3) ON CONFLICT(token_hash) DO NOTHING`, hash, username, expires); err != nil {
		return err
	}
	var revoked bool
	if err = tx.QueryRowContext(ctx, `SELECT revoked FROM auth_sessions WHERE token_hash=$1`, hash).Scan(&revoked); err != nil {
		return err
	}
	if revoked {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (d *DB) RevokeAuthSession(ctx context.Context, hash, username string, expires time.Time) error {
	_, err := d.ExecContext(ctx, `INSERT INTO auth_sessions(token_hash,username,expires_at,revoked)
        VALUES($1,$2,$3,true) ON CONFLICT(token_hash) DO UPDATE SET revoked=true`, hash, username, expires)
	return err
}

// ChangeAuthPassword uses the same settings-row lock as session issuance. A
// login verified against an old password cannot issue a new session afterward.
func (d *DB) ChangeAuthPassword(ctx context.Context, key, oldHash, newHash string) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE settings SET value=$1,updated_at=now() WHERE key=$2 AND value=$3`, newHash, key, oldHash)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("password changed; sign in again")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET revoked=true WHERE NOT revoked`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES('auth.sessions.legacy_disabled','true')
        ON CONFLICT(key) DO UPDATE SET value='true',updated_at=now()`); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) IssueAuthSession(ctx context.Context, passwordKey, verifiedHash, tokenHash, username string, expires time.Time) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current string
	if err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1 FOR SHARE`, passwordKey).Scan(&current); err != nil {
		return err
	}
	if current != verifiedHash {
		return sql.ErrNoRows
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO auth_sessions(token_hash,username,expires_at) VALUES($1,$2,$3)`, tokenHash, username, expires); err != nil {
		return err
	}
	return tx.Commit()
}
