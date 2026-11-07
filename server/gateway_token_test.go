package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/afterglow-labs/afterglow-cybersecurity/db"
	"github.com/golang-jwt/jwt/v5"
)

func TestLegacyOdysseusGatewayTokensAreRejected(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	server := &Server{jwtKey: key, authSessions: &memoryAuthSessions{}}
	protected := server.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, identity := range []struct{ subject, audience string }{
		{"odysseus-gateway", "afterglow-api"},
		{"odysseus-gateway", gatewayAudience},
		{gatewaySubject, "afterglow-api"},
	} {
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
			Subject: identity.subject, Audience: jwt.ClaimStrings{identity.audience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		}).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		protected.ServeHTTP(response, req)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("legacy gateway identity accepted: %+v status=%d", identity, response.Code)
		}
	}
}

func TestGatewayTokenCannotBecomeAdministrator(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	admin, err := signJWT(key)
	if err != nil {
		t.Fatal(err)
	}
	gateway, _, err := signGatewayJWT(key)
	if err != nil {
		t.Fatal(err)
	}
	if verifyJWT(gateway, key) || !verifyGatewayJWT(gateway, key) {
		t.Fatal("gateway token must be valid only for its own subject")
	}
	if !verifyJWT(admin, key) || verifyGatewayJWT(admin, key) {
		t.Fatal("administrator token must retain its original role")
	}

	server := &Server{jwtKey: key, authSessions: &memoryAuthSessions{}}
	protected := server.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		name, method, target, token string
		cookie                      bool
		want                        int
	}{
		{"admin settings", "GET", "/api/settings", admin, false, 204},
		{"gateway status", "GET", "/api/stats", gateway, false, 204},
		{"gateway goal edit", "PATCH", "/api/tasks/7/goals/2", gateway, false, 204},
		{"gateway learning outcome", "GET", "/api/tasks/7/outcome", gateway, false, 204},
		{"gateway learning lessons", "GET", "/api/tasks/7/lessons", gateway, false, 204},
		{"gateway lesson extraction", "POST", "/api/tasks/7/lessons/extract", gateway, false, 204},
		{"gateway follow-up waves", "GET", "/api/tasks/7/waves", gateway, false, 204},
		{"gateway evidence artifacts", "GET", "/api/tasks/7/artifacts", gateway, false, 204},
		{"gateway handoff metadata", "GET", "/api/tasks/7/c2-handoffs/summary", gateway, false, 204},
		{"gateway business map", "GET", "/api/tasks/7/business-map", gateway, false, 204},
		{"gateway campaign", "GET", "/api/tasks/7/campaign", gateway, false, 204},
		{"gateway hunt classes", "GET", "/api/tasks/7/hunt-classes", gateway, false, 204},
		{"gateway attack chains", "GET", "/api/tasks/7/attack-chains", gateway, false, 204},
		{"gateway model test needs safe body", "POST", "/api/llm/test", gateway, false, 403},
		{"gateway chain compose denied", "POST", "/api/tasks/7/attack-chains/compose", gateway, false, 403},
		{"gateway hunt edit denied", "PUT", "/api/tasks/7/hunt-classes/auth", gateway, false, 403},
		{"gateway handoff status denied", "POST", "/api/tasks/7/c2-handoffs/2/status", gateway, false, 403},
		{"gateway raw handoffs denied", "GET", "/api/tasks/7/c2-handoffs", gateway, false, 403},
		{"gateway settings denied", "GET", "/api/settings", gateway, false, 403},
		{"gateway credential reveal denied", "GET", "/api/tasks/7/credentials/1/secret", gateway, false, 403},
		{"gateway profile delete denied", "DELETE", "/api/llm/profiles/2", gateway, false, 403},
		{"gateway wrong method denied", "DELETE", "/api/tasks/7/goals", gateway, false, 403},
		{"gateway nonnumeric task denied", "GET", "/api/tasks/category", gateway, false, 403},
		{"gateway cookie denied", "GET", "/api/stats", gateway, true, 401},
		{"gateway query denied", "GET", "/api/stats?token=" + gateway, "", false, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.target, nil)
			if tc.cookie {
				r.AddCookie(&http.Cookie{Name: "afterglow_token", Value: tc.token})
			} else if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			protected.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}

	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject: gatewaySubject, Audience: jwt.ClaimStrings{gatewayAudience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
	}).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	if verifyGatewayJWT(expired, key) {
		t.Fatal("expired gateway token was accepted")
	}
	noExpiry, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject: gatewaySubject, Audience: jwt.ClaimStrings{gatewayAudience},
	}).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	if verifyGatewayJWT(noExpiry, key) {
		t.Fatal("gateway token without expiration was accepted")
	}
}

func TestGatewayC2HandoffSummaryOmitsShellURL(t *testing.T) {
	rows := []db.C2Handoff{{
		ID: 7, Vector: "webshell_upload", TargetHost: "lab.test",
		TargetPath: "/test", ShellURL: "https://secret.test/token", Status: "requested",
	}}
	encoded, err := json.Marshal(summarizeC2Handoffs(rows))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "shell_url") || strings.Contains(string(encoded), "secret.test") {
		t.Fatalf("scoped handoff summary disclosed shell URL: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"id":"7"`) || !strings.Contains(string(encoded), `"status":"requested"`) {
		t.Fatalf("scoped handoff summary lost status metadata: %s", encoded)
	}
}

func TestGatewayModelPayloadsStayOnApprovedTargets(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	token, _, err := signGatewayJWT(key)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{jwtKey: key, authSessions: &memoryAuthSessions{}}
	var forwardedBody string
	protected := server.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(strings.Builder)
		if _, err := io.Copy(body, r.Body); err != nil {
			t.Fatal(err)
		}
		forwardedBody = body.String()
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		name, path, body string
		want             int
	}{
		{"local discovery", "/api/llm/models", `{"provider":"ollama","base_url":"http://ollama:11434"}`, 204},
		{"discovery SSRF denied", "/api/llm/models", `{"provider":"ollama","base_url":"http://external.example"}`, 403},
		{"discovery API key denied", "/api/llm/models", `{"provider":"ollama","base_url":"http://ollama:11434","api_key":"secret"}`, 403},
		{"local test", "/api/llm/test", `{"profile_id":1,"provider":"ollama","model":"qwen3:32b","base_url":"http://127.0.0.1:11434","streaming":true,"context_window_k":32}`, 204},
		{"test proxy denied", "/api/llm/test", `{"profile_id":1,"provider":"ollama","model":"qwen3:32b","base_url":"http://127.0.0.1:11434","streaming":true,"context_window_k":32,"proxy":"http://external.example"}`, 403},
		{"local profile", "/api/llm/profiles", `{"name":"Local","format":"ollama","model":"qwen3:32b","base_url":"http://ollama:11434","streaming":true,"context_window_k":32,"max_tokens":4096,"keep_alive":"30m","llm_max_concurrent":1}`, 204},
		{"profile API key denied", "/api/llm/profiles", `{"name":"Local","format":"ollama","model":"qwen3:32b","base_url":"http://ollama:11434","streaming":true,"context_window_k":32,"max_tokens":4096,"keep_alive":"30m","llm_max_concurrent":1,"api_key":"secret"}`, 403},
		{"ChatGPT plan profile", "/api/llm/profiles", `{"name":"Plan","format":"openai-responses","auth_mode":"chatgpt_plan","chatgpt_account_id":"account-1","model":"test-model","pool_exclude":true,"streaming":true}`, 204},
		{"arbitrary Responses profile denied", "/api/llm/profiles", `{"name":"Plan","format":"openai-responses","auth_mode":"api_key","chatgpt_account_id":"account-1","model":"test-model","pool_exclude":true,"streaming":true}`, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forwardedBody = ""
			r := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			protected.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.want, w.Body.String())
			}
			if tc.want == 204 && forwardedBody != tc.body {
				t.Fatalf("allowed body changed before handler: %q", forwardedBody)
			}
			if tc.want == 403 && forwardedBody != "" {
				t.Fatal("rejected model request reached its handler")
			}
		})
	}
}

func TestGatewayTokenIssueRequiresAdminBearer(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	admin, err := signJWT(key)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{jwtKey: key, authSessions: &memoryAuthSessions{}}
	request := func(token string, cookie bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/auth/gateway-token", strings.NewReader("{}"))
		if cookie {
			r.AddCookie(&http.Cookie{Name: "afterglow_token", Value: token})
		} else if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		server.issueGatewayToken(w, r)
		return w
	}
	if w := request(admin, true); w.Code != 401 {
		t.Fatalf("admin cookie issued a gateway token: %d", w.Code)
	}
	if w := request("", false); w.Code != 401 {
		t.Fatalf("anonymous request issued a gateway token: %d", w.Code)
	}
	w := request(admin, false)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("admin bearer issuance: status=%d cache=%q", w.Code, w.Header().Get("Cache-Control"))
	}
	var result struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !verifyGatewayJWT(result.Token, key) || verifyJWT(result.Token, key) || result.ExpiresAt == "" {
		t.Fatal("issued token lacked the required gateway scope or expiration")
	}
	if denied := request(result.Token, false); denied.Code != 401 {
		t.Fatalf("gateway token minted another token: %d", denied.Code)
	}
}

func TestGatewayPathMatchingRejectsEscapesAndTraversal(t *testing.T) {
	for _, target := range []string{
		"/api/tasks/7/../settings", "/api/tasks//7", "/api/tasks/0/goals",
		"/api/tasks/7%2Fcredentials", "/api/tasks/7/goals/../../settings",
	} {
		r := httptest.NewRequest("GET", target, nil)
		if gatewayRouteAllowed(r) {
			t.Fatalf("gateway token allowed %q", target)
		}
	}
}
