package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/afterglow-labs/afterglow-cybersecurity/db"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Explicit in-memory repository for database-independent authorization tests.
// Production has no fallback: it uses PostgreSQL and fails closed on errors.
type memoryAuthSessions struct {
	mu       sync.Mutex
	rows     map[string]db.AuthSession
	disabled bool
}

func (m *memoryAuthSessions) AuthSessionPolicy(_ context.Context, hash string) (*db.AuthSession, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[hash]
	if !ok {
		return nil, !m.disabled, nil
	}
	return &row, !m.disabled, nil
}
func (m *memoryAuthSessions) RegisterAuthSession(_ context.Context, hash, user string, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows == nil {
		m.rows = map[string]db.AuthSession{}
	}
	if _, ok := m.rows[hash]; !ok {
		m.rows[hash] = db.AuthSession{Username: user, ExpiresAt: expires}
	}
	return nil
}
func (m *memoryAuthSessions) RestoreLegacyAuthSession(ctx context.Context, hash, user string, expires time.Time) error {
	return m.RegisterAuthSession(ctx, hash, user, expires)
}
func (m *memoryAuthSessions) RevokeAuthSession(_ context.Context, hash, user string, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows == nil {
		m.rows = map[string]db.AuthSession{}
	}
	m.rows[hash] = db.AuthSession{Username: user, ExpiresAt: expires, Revoked: true}
	return nil
}
func (m *memoryAuthSessions) IssueAuthSession(ctx context.Context, _, _, hash, user string, expires time.Time) error {
	return m.RegisterAuthSession(ctx, hash, user, expires)
}

const platformTestPassword = "synthetic-password-only-42"

func platformTestServer(t *testing.T) *Server {
	t.Helper()
	m := backportTestManager(t)
	for _, key := range []string{authPassKey, "auth.sessions.legacy_disabled"} {
		value, exists, err := m.pg.GetSetting(key)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if exists {
				_ = m.pg.SetSetting(key, value)
			} else {
				_, _ = m.pg.Exec(`DELETE FROM settings WHERE key=$1`, key)
			}
		})
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(platformTestPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.pg.SetSetting(authPassKey, string(hash)); err != nil {
		t.Fatal(err)
	}
	if err = m.pg.SetSetting("auth.sessions.legacy_disabled", "false"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return &Server{m: m, jwtKey: key, ctx: context.Background(), engine: NewEngine(m), chatBusy: map[string]bool{}}
}

func platformRequest(s *Server, method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://aptex.test"+path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func platformLogin(t *testing.T, s *Server, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": "Afterglow", "password": password})
	w := platformRequest(s, "POST", "/api/v1/auth/login", "", string(body))
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	var reply struct{ Token string }
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	claims, valid := parseJWTClaims(reply.Token, s.jwtKey)
	if !valid || claims.ID == "" {
		t.Fatal("new device session has no unique identity")
	}
	return reply.Token
}

func TestClientSessionsRevokeOneDeviceAndPersistOnlyDigests(t *testing.T) {
	s := platformTestServer(t)
	one, two := platformLogin(t, s, platformTestPassword), platformLogin(t, s, platformTestPassword)
	if one == two {
		t.Fatal("two devices received the same credential")
	}
	var count int
	if err := s.m.pg.QueryRow(`SELECT count(*) FROM auth_sessions WHERE token_hash=$1`, authTokenHash(one)).Scan(&count); err != nil || count != 1 {
		t.Fatalf("digest not stored: %v %d", err, count)
	}
	if err := s.m.pg.QueryRow(`SELECT count(*) FROM auth_sessions WHERE token_hash=$1`, one).Scan(&count); err != nil || count != 0 {
		t.Fatal("raw credential stored")
	}
	if w := platformRequest(s, "POST", "/api/v1/auth/logout", one, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := platformRequest(s, "GET", "/api/v1/auth/session", one, ""); w.Code != 401 {
		t.Fatalf("revoked session accepted: %d", w.Code)
	}
	if w := platformRequest(s, "GET", "/api/v1/auth/session", two, ""); w.Code != 200 {
		t.Fatalf("other device lost its session: %d", w.Code)
	}
	if w := platformRequest(s, "POST", "/api/auth/gateway-token", one, "{}"); w.Code != 401 {
		t.Fatal("revoked session minted service credential")
	}
}

func TestClientSessionPasswordChangeRevokesNewAndLegacyCredentials(t *testing.T) {
	s := platformTestServer(t)
	one, two := platformLogin(t, s, platformTestPassword), platformLogin(t, s, platformTestPassword)
	oldHash, _, _ := s.m.pg.GetSetting(authPassKey)
	legacy, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	if w := platformRequest(s, "GET", "/api/v1/auth/session", legacy, ""); w.Code != 200 {
		t.Fatal("legacy transition failed")
	}
	body := `{"old_password":"` + platformTestPassword + `","new_password":"new-synthetic-password"}`
	if w := platformRequest(s, "POST", "/api/auth/change-password", one, body); w.Code != 200 {
		t.Fatalf("change: %d %s", w.Code, w.Body.String())
	}
	for _, token := range []string{one, two, legacy} {
		if w := platformRequest(s, "GET", "/api/v1/auth/session", token, ""); w.Code != 401 {
			t.Fatalf("old credential accepted: %d", w.Code)
		}
	}
	if _, _, err = s.issueAdminSession(context.Background(), oldHash); err == nil {
		t.Fatal("password-change race allowed old verifier to issue a credential")
	}
	if _, err = s.validateAdminSession(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	_ = platformLogin(t, s, "new-synthetic-password")
}

func TestClientSessionCookiesOriginsAndURLCredentials(t *testing.T) {
	s := platformTestServer(t)
	r := httptest.NewRequest("POST", "http://aptex.test/api/v1/auth/login", strings.NewReader(`{"username":"Afterglow","password":"`+platformTestPassword+`"}`))
	r.Header.Set("Origin", "https://aptex.test")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("insecure browser session cookie")
	}
	token := platformLogin(t, s, platformTestPassword)
	if w = platformRequest(s, "GET", "/api/v1/auth/session?token="+token, "", ""); w.Code != 400 {
		t.Fatal("v1 accepted credential in URL")
	}
	request := httptest.NewRequest("POST", "http://aptex.test/api/v1/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: "afterglow_token", Value: token})
	request.Header.Set("Origin", "https://attacker.invalid")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, request)
	if w.Code != 403 {
		t.Fatal("cross-origin cookie mutation accepted")
	}
	if w = platformRequest(s, "GET", "/api/v1/auth/session", token, ""); w.Code != 200 {
		t.Fatal("rejected attack revoked the session")
	}
	forged, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: adminUsername(), ID: "unregistered", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}).SignedString(s.jwtKey)
	if w = platformRequest(s, "GET", "/api/v1/auth/session", forged, ""); w.Code != 401 {
		t.Fatal("unregistered new session accepted")
	}
}

func TestClientMetaReadinessAndVersionedRoutes(t *testing.T) {
	s := platformTestServer(t)
	for _, path := range []string{"/api/v1/meta", "/api/ready"} {
		if w := platformRequest(s, "GET", path, "", ""); w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	if w := platformRequest(s, "GET", "/api/v1/capabilities", "", ""); w.Code != 401 {
		t.Fatal("capabilities exposed without a session")
	}
	token := platformLogin(t, s, platformTestPassword)
	for _, path := range []string{"/api/v1/capabilities", "/api/v1/tasks", "/api/v1/llm/profiles"} {
		if w := platformRequest(s, "GET", path, token, ""); w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	if err := s.m.pg.Close(); err != nil {
		t.Fatal(err)
	}
	if w := platformRequest(s, "GET", "/api/ready", "", ""); w.Code != 503 {
		t.Fatal("readiness ignored failed database")
	}
	if w := platformRequest(s, "GET", "/api/v1/auth/session", token, ""); w.Code != 503 {
		t.Fatal("sessions failed open without storage")
	}
	if w := platformRequest(s, "GET", "/api/health", "", ""); w.Code != 200 {
		t.Fatal("liveness was coupled to readiness")
	}
}

func TestClientSessionRevocationClosesLegacyObservers(t *testing.T) {
	s := platformTestServer(t)
	token := platformLogin(t, s, platformTestPassword)
	closed := make(chan struct{})
	observer := s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	server := httptest.NewServer(observer)
	defer server.Close()
	request, _ := http.NewRequest("GET", server.URL+"/api/logs/stream", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := (&http.Client{Timeout: 8 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if w := platformRequest(s, "POST", "/api/v1/auth/logout", token, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, err = io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("revoked legacy observer stayed open")
	}
}
