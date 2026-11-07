package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
)

const (
	gatewaySubject  = "aptex-gateway"
	gatewayAudience = "aptex-api"
	gatewayTokenTTL = 30 * 24 * time.Hour
)

func signGatewayJWT(key []byte) (string, time.Time, error) {
	expires := time.Now().Add(gatewayTokenTTL)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   gatewaySubject,
		Audience:  jwt.ClaimStrings{gatewayAudience},
		ExpiresAt: jwt.NewNumericDate(expires),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}).SignedString(key)
	return token, expires, err
}

func verifyGatewayJWT(token string, key []byte) bool {
	claims, valid := parseJWTClaims(token, key)
	if !valid || claims.Subject != gatewaySubject {
		return false
	}
	for _, audience := range claims.Audience {
		if audience == gatewayAudience {
			return true
		}
	}
	return false
}

// issueGatewayToken requires an administrator bearer token. The returned token
// has a distinct APTEX subject and can call only the scoped integration routes.
func (s *Server) issueGatewayToken(w http.ResponseWriter, r *http.Request) {
	adminToken, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		writeErr(w, 401, "Administrator bearer token required")
		return
	}
	claims, sessionErr := s.validateAdminSession(r.Context(), adminToken)
	if sessionErr != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if claims == nil {
		writeErr(w, 401, "Administrator bearer token required")
		return
	}
	token, expires, err := signGatewayJWT(s.jwtKey)
	if err != nil {
		writeErr(w, 500, "Could not issue gateway token")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"token": token, "expires_at": expires.UTC().Format(time.RFC3339),
	})
}

type gatewayRoute struct{ method, pattern string }

// These are the exact backend methods and paths available to scoped clients.
// Adding a route here is a privilege expansion and requires review alongside
// the client handler that calls it.
var gatewayRoutes = []gatewayRoute{
	{"GET", "/api/stats"},
	{"GET", "/api/tasks"},
	{"POST", "/api/tasks"},
	{"GET", "/api/tasks/{id}"},
	{"POST", "/api/tasks/{id}/control"},
	{"GET", "/api/tasks/{id}/goals"},
	{"POST", "/api/tasks/{id}/goals"},
	{"PATCH", "/api/tasks/{id}/goals/{gid}"},
	{"DELETE", "/api/tasks/{id}/goals/{gid}"},
	{"GET", "/api/tasks/{id}/constraints"},
	{"POST", "/api/tasks/{id}/constraints"},
	{"PATCH", "/api/tasks/{id}/constraints/{cid}"},
	{"DELETE", "/api/tasks/{id}/constraints/{cid}"},
	{"GET", "/api/tasks/{id}/outcome"},
	{"GET", "/api/tasks/{id}/lessons"},
	{"POST", "/api/tasks/{id}/lessons/extract"},
	{"GET", "/api/tasks/{id}/waves"},
	{"GET", "/api/tasks/{id}/artifacts"},
	{"GET", "/api/tasks/{id}/c2-handoffs/summary"},
	{"GET", "/api/tasks/{id}/business-map"},
	{"GET", "/api/tasks/{id}/campaign"},
	{"GET", "/api/tasks/{id}/hunt-classes"},
	{"GET", "/api/tasks/{id}/attack-chains"},
	{"GET", "/api/tasks/{id}/chat/status"},
	{"POST", "/api/chat"},
	{"POST", "/api/tasks/{id}/chat/stop"},
	{"POST", "/api/exploration/main-session/new"},
	{"GET", "/api/tasks/{id}/coverage"},
	{"GET", "/api/tasks/{id}/scope"},
	{"POST", "/api/tasks/{id}/scope"},
	{"DELETE", "/api/tasks/{id}/scope/{sid}"},
	{"GET", "/api/tasks/{id}/intercept-rules"},
	{"POST", "/api/tasks/{id}/intercept-rules"},
	{"POST", "/api/tasks/{id}/intercept-rules/{rid}/toggle"},
	{"DELETE", "/api/tasks/{id}/intercept-rules/{rid}"},
	{"GET", "/api/exploration/activity/history"},
	{"GET", "/api/exploration/findings"},
	{"GET", "/api/exploration/findings/{id}"},
	{"PATCH", "/api/exploration/findings/{id}"},
	{"GET", "/api/assets"},
	{"POST", "/api/assets"},
	{"GET", "/api/llm"},
	{"GET", "/api/llm/profiles"},
	{"POST", "/api/llm/profiles"},
	{"POST", "/api/llm/profiles/active"},
	{"GET", "/api/llm/pool"},
	{"POST", "/api/llm/models"},
	{"POST", "/api/llm/test"},
	{"GET", "/api/chatgpt/status"},
	{"POST", "/api/chatgpt/login"},
	{"POST", "/api/chatgpt/logout"},
	{"GET", "/api/chatgpt/models"},
	{"POST", "/api/chatgpt/recheck"},
	{"GET", "/api/intercept/pending"},
	{"POST", "/api/intercept/pending/{id}/decide"},
	{"GET", "/api/traffic"},
	{"GET", "/api/traffic/exchange"},
	{"GET", "/api/report"},
}

func gatewayRouteAllowed(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.RawPath != "" ||
		r.URL.EscapedPath() != r.URL.Path || path.Clean(r.URL.Path) != r.URL.Path {
		return false
	}
	actual := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	for _, route := range gatewayRoutes {
		if r.Method != route.method {
			continue
		}
		wanted := strings.Split(strings.TrimPrefix(route.pattern, "/"), "/")
		if len(actual) != len(wanted) {
			continue
		}
		match := true
		for i, segment := range wanted {
			if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
				n, err := strconv.ParseUint(actual[i], 10, 64)
				if err != nil || n == 0 {
					match = false
					break
				}
			} else if actual[i] != segment {
				match = false
				break
			}
		}
		if match {
			return gatewayModelBodyAllowed(r)
		}
	}
	return false
}

var gatewayOllamaEndpoints = map[string]bool{
	"http://127.0.0.1:11434":            true,
	"http://host.docker.internal:11434": true,
	"http://ollama:11434":               true,
}

func exactGatewayFields(fields map[string]json.RawMessage, names ...string) bool {
	if len(fields) != len(names) {
		return false
	}
	for _, name := range names {
		if _, ok := fields[name]; !ok {
			return false
		}
	}
	return true
}

func gatewayText(value string, max int) bool {
	return value != "" && value == strings.TrimSpace(value) && utf8.RuneCountInString(value) <= max
}

// The three model setup routes can otherwise point a valid gateway token at
// arbitrary network targets or save an API credential. Accept only the exact
// local Ollama and ChatGPT-plan payloads constructed by scoped clients.
func gatewayModelBodyAllowed(r *http.Request) bool {
	if r.Method != http.MethodPost || (r.URL.Path != "/api/llm/models" &&
		r.URL.Path != "/api/llm/test" && r.URL.Path != "/api/llm/profiles") {
		return true
	}
	const maxBody = 16 * 1024
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil || len(raw) > maxBody {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	switch r.URL.Path {
	case "/api/llm/models":
		if !exactGatewayFields(fields, "provider", "base_url") {
			return false
		}
		var body struct {
			Provider string `json:"provider"`
			BaseURL  string `json:"base_url"`
		}
		return json.Unmarshal(raw, &body) == nil && body.Provider == "ollama" && gatewayOllamaEndpoints[body.BaseURL]
	case "/api/llm/test":
		if !exactGatewayFields(fields, "profile_id", "provider", "model", "base_url", "streaming", "context_window_k") {
			return false
		}
		var body struct {
			ProfileID      int64  `json:"profile_id"`
			Provider       string `json:"provider"`
			Model          string `json:"model"`
			BaseURL        string `json:"base_url"`
			Streaming      bool   `json:"streaming"`
			ContextWindowK int    `json:"context_window_k"`
		}
		return json.Unmarshal(raw, &body) == nil && body.ProfileID > 0 && body.Provider == "ollama" &&
			gatewayText(body.Model, 200) && gatewayOllamaEndpoints[body.BaseURL] && body.Streaming && body.ContextWindowK == 32
	case "/api/llm/profiles":
		var format struct {
			Format string `json:"format"`
		}
		if json.Unmarshal(raw, &format) != nil {
			return false
		}
		switch format.Format {
		case "ollama":
			if !exactGatewayFields(fields, "name", "format", "model", "base_url", "streaming",
				"context_window_k", "max_tokens", "keep_alive", "llm_max_concurrent") {
				return false
			}
			var body struct {
				Name           string `json:"name"`
				Format         string `json:"format"`
				Model          string `json:"model"`
				BaseURL        string `json:"base_url"`
				Streaming      bool   `json:"streaming"`
				ContextWindowK int    `json:"context_window_k"`
				MaxTokens      int    `json:"max_tokens"`
				KeepAlive      string `json:"keep_alive"`
				MaxConcurrent  int    `json:"llm_max_concurrent"`
			}
			return json.Unmarshal(raw, &body) == nil && gatewayText(body.Name, 160) && gatewayText(body.Model, 200) &&
				gatewayOllamaEndpoints[body.BaseURL] && body.Streaming && body.ContextWindowK == 32 &&
				body.MaxTokens == 4096 && body.KeepAlive == "30m" && body.MaxConcurrent == 1
		case "openai-responses":
			if !exactGatewayFields(fields, "name", "format", "auth_mode", "chatgpt_account_id", "model", "pool_exclude", "streaming") {
				return false
			}
			var body struct {
				Name             string `json:"name"`
				Format           string `json:"format"`
				AuthMode         string `json:"auth_mode"`
				ChatGPTAccountID string `json:"chatgpt_account_id"`
				Model            string `json:"model"`
				PoolExclude      bool   `json:"pool_exclude"`
				Streaming        bool   `json:"streaming"`
			}
			return json.Unmarshal(raw, &body) == nil && gatewayText(body.Name, 160) &&
				gatewayText(body.ChatGPTAccountID, 200) && gatewayText(body.Model, 200) &&
				body.AuthMode == "chatgpt_plan" && body.PoolExclude && body.Streaming
		}
	}
	return false
}
