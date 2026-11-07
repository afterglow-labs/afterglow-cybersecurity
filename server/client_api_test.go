package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/afterglow-labs/afterglow-cybersecurity/db"
)

func nextClientEvent(t *testing.T, reader *bufio.Scanner, want string) (string, map[string]any) {
	t.Helper()
	event, id, data := "", "", ""
	for reader.Scan() {
		line := reader.Text()
		if strings.HasPrefix(line, "event: ") {
			event = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "id: ") {
			id = strings.TrimPrefix(line, "id: ")
		}
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
		}
		if line == "" {
			if event == want {
				var value map[string]any
				if err := json.Unmarshal([]byte(data), &value); err != nil {
					t.Fatal(err)
				}
				return id, value
			}
			event, id, data = "", "", ""
		}
	}
	t.Fatalf("stream ended before %s: %v", want, reader.Err())
	return "", nil
}

func TestClientEventsResumeFromPersistedCursorAndHonorRevocation(t *testing.T) {
	s := platformTestServer(t)
	token := platformLogin(t, s, platformTestPassword)
	conversation, err := s.m.pg.CreateConversation("mainagent", "native client synthetic fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.m.pg.AppendConvActivity(conversation.ID, db.Activity{Worker: "assistant", Kind: "text", Summary: "first synthetic reply"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	client := &http.Client{Timeout: 6 * time.Second}
	address := server.URL + "/api/v1/conversations/" + strconv.FormatInt(conversation.ID, 10) + "/events"
	open := func(cursor string) *http.Response {
		request, _ := http.NewRequest("GET", address, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		if cursor != "" {
			request.Header.Set("Last-Event-ID", cursor)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 200 {
			response.Body.Close()
			t.Fatalf("stream: %d", response.StatusCode)
		}
		if response.Header.Get("Content-Type") != "text/event-stream" || response.Header.Get("Content-Encoding") != "" {
			t.Fatal("stream was buffered or encoded")
		}
		return response
	}
	response := open("")
	id, body := nextClientEvent(t, bufio.NewScanner(response.Body), "activity")
	if id != strconv.FormatInt(first, 10) || body["summary"] != "first synthetic reply" {
		t.Fatal("wrong initial event")
	}
	response.Body.Close()
	second, err := s.m.pg.AppendConvActivity(conversation.ID, db.Activity{Worker: "assistant", Kind: "text", Summary: "second synthetic reply"})
	if err != nil {
		t.Fatal(err)
	}
	response = open(id)
	defer response.Body.Close()
	reader := bufio.NewScanner(response.Body)
	resumed, body := nextClientEvent(t, reader, "activity")
	if resumed != strconv.FormatInt(second, 10) || body["summary"] != "second synthetic reply" {
		t.Fatal("reconnect replayed old data or skipped persisted data")
	}
	if w := platformRequest(s, "POST", "/api/v1/auth/logout", token, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	_, expired := nextClientEvent(t, reader, "session_expired")
	if expired["reauthenticate"] != true {
		t.Fatal("revoked stream remained authorized")
	}
}

func TestClientOriginsRequireExplicitCredentialedAllowlist(t *testing.T) {
	t.Setenv("APTEX_ALLOWED_ORIGINS", "http://localhost:3000")
	for _, tc := range []struct {
		origin  string
		allowed bool
	}{
		{"http://localhost:3000", true}, {"http://localhost:3001", false}, {"https://attacker.invalid", false}, {"null", false},
	} {
		request := httptest.NewRequest("GET", "http://localhost:8787/api/health", nil)
		request.Header.Set("Origin", tc.origin)
		response := httptest.NewRecorder()
		cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(response, request)
		got := response.Header().Get("Access-Control-Allow-Credentials") == "true"
		if got != tc.allowed {
			t.Fatalf("origin %q credentials=%v", tc.origin, got)
		}
		if got && response.Header().Get("Access-Control-Allow-Origin") != tc.origin {
			t.Fatal("credentialed CORS used a wildcard")
		}
	}
}
