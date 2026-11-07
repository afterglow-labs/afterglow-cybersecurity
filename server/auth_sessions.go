package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/afterglow-labs/afterglow-cybersecurity/db"
	"github.com/afterglow-labs/afterglow-cybersecurity/locale"
	"github.com/golang-jwt/jwt/v5"
)

type authSessionRepository interface {
	AuthSessionPolicy(context.Context, string) (*db.AuthSession, bool, error)
	RegisterAuthSession(context.Context, string, string, time.Time) error
	RestoreLegacyAuthSession(context.Context, string, string, time.Time) error
	RevokeAuthSession(context.Context, string, string, time.Time) error
	IssueAuthSession(context.Context, string, string, string, string, time.Time) error
}

func (s *Server) sessionRepository() authSessionRepository {
	if s.authSessions != nil {
		return s.authSessions
	}
	if s.m != nil && s.m.pg != nil {
		return s.m.pg
	}
	return nil
}

func authTokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// Older signed administrator JWTs keep their remaining lifetime during the
// transition. Revocation records take precedence; password changes disable all
// unregistered legacy tokens. New sessions always have a unique ID and a row.
func (s *Server) validateAdminSession(ctx context.Context, token string) (*jwt.RegisteredClaims, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	claims, valid := parseJWTClaims(token, s.jwtKey)
	if !valid || claims.Subject != adminUsername() || claims.Issuer != "" || len(claims.Audience) != 0 {
		return nil, nil
	}
	store := s.sessionRepository()
	if store == nil {
		return nil, errors.New("session data source unavailable")
	}
	row, legacyAllowed, err := store.AuthSessionPolicy(ctx, authTokenHash(token))
	if err != nil {
		return nil, err
	}
	if row != nil {
		if row.Revoked || row.Username != adminUsername() || !row.ExpiresAt.After(time.Now()) {
			return nil, nil
		}
		return claims, nil
	}
	if claims.ID == "" && legacyAllowed && claims.IssuedAt != nil && !claims.IssuedAt.After(time.Now().Add(30*time.Second)) {
		return claims, nil
	}
	return nil, nil
}

func (s *Server) issueAdminSession(ctx context.Context, verifiedHash string) (string, time.Time, error) {
	id := make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(jwtTTL).Truncate(time.Second)
	claims := jwt.RegisteredClaims{Subject: adminUsername(), ID: hex.EncodeToString(id),
		IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(expires)}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtKey)
	if err != nil {
		return "", time.Time{}, err
	}
	store := s.sessionRepository()
	if store == nil {
		return "", time.Time{}, errors.New("session data source unavailable")
	}
	err = store.IssueAuthSession(ctx, authPassKey, verifiedHash, authTokenHash(token), adminUsername(), expires)
	return token, expires, err
}

func sessionCookieSecure(r *http.Request) bool {
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	// Browsers send Origin on login/restore even when TLS terminates at Serve.
	origin, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && origin.Scheme == "https" && strings.EqualFold(origin.Host, r.Host)
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: "afterglow_token", Value: token, Path: "/", HttpOnly: true,
		Secure: sessionCookieSecure(r), SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "afterglow_token", Path: "/", HttpOnly: true, Secure: sessionCookieSecure(r),
		SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}

func (s *Server) authSession(w http.ResponseWriter, r *http.Request) {
	claims, err := s.validateAdminSession(r.Context(), extractToken(r))
	if err != nil {
		writeErr(w, 503, locale.Text(responseLanguage(w), errDataSourceUnavailable))
		return
	}
	if claims == nil {
		writeErr(w, 401, "Invalid or expired session")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"user": map[string]string{"id": "administrator", "username": claims.Subject, "name": claims.Subject},
		"expires_at": claims.ExpiresAt.Time.UTC().Format(time.RFC3339), "api_version": "1"})
}

func (s *Server) authRestore(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	claims, err := s.validateAdminSession(r.Context(), token)
	if err != nil {
		writeErr(w, 503, locale.Text(responseLanguage(w), errDataSourceUnavailable))
		return
	}
	if claims == nil {
		writeErr(w, 401, "Invalid or expired session")
		return
	}
	if claims.ID == "" {
		if err = s.sessionRepository().RestoreLegacyAuthSession(r.Context(), authTokenHash(token), claims.Subject, claims.ExpiresAt.Time); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeErr(w, 401, "Session changed; sign in again")
			} else {
				writeErr(w, 503, locale.Text(responseLanguage(w), errDataSourceUnavailable))
			}
			return
		}
	}
	setSessionCookie(w, r, token, claims.ExpiresAt.Time)
	s.authSession(w, r)
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	claims, err := s.validateAdminSession(r.Context(), token)
	if err != nil {
		writeErr(w, 503, locale.Text(responseLanguage(w), errDataSourceUnavailable))
		return
	}
	if claims != nil {
		if err = s.sessionRepository().RevokeAuthSession(r.Context(), authTokenHash(token), claims.Subject, claims.ExpiresAt.Time); err != nil {
			writeErr(w, 503, locale.Text(responseLanguage(w), errDataSourceUnavailable))
			return
		}
	}
	clearSessionCookie(w, r)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// Long-lived legacy observers must stop when their device session is revoked.
// Cancelling an HTTP observer never cancels the server-owned execution context.
func (s *Server) serveSessionObserver(h http.Handler, w http.ResponseWriter, r *http.Request, token string) {
	path := r.URL.Path
	if !strings.HasSuffix(path, "/stream") && !strings.HasSuffix(path, "/events") {
		h.ServeHTTP(w, r)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				claims, err := s.validateAdminSession(ctx, token)
				if err != nil || claims == nil {
					cancel()
					return
				}
			}
		}
	}()
	h.ServeHTTP(w, r.WithContext(ctx))
}

// Cookie-authenticated mutations are same-origin. Bearer clients do not rely on
// ambient browser credentials; rejected origins never gain access via cookies.
func sameOriginMutation(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	if allowedClientOrigin(origin) {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.User == nil && parsed.Path == "" && strings.EqualFold(parsed.Host, r.Host)
}

func allowedClientOrigin(origin string) bool {
	if origin == "" || origin == "null" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return false
	}
	for _, allowed := range strings.Split(os.Getenv("APTEX_ALLOWED_ORIGINS"), ",") {
		if strings.EqualFold(strings.TrimSpace(allowed), origin) {
			return true
		}
	}
	return false
}
