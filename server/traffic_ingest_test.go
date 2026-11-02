package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/traffic"
)

func TestBrowserCaptureIngestUsesTrafficStoreAndAuth(t *testing.T) {
	tr, err := traffic.Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	s := &Server{m: &Manager{traffic: tr}, jwtKey: []byte("test-browser-capture-signing-key")}
	h := cors(s.requireAuth(http.HandlerFunc(s.ingestTraffic)))

	preflight := httptest.NewRequest(http.MethodOptions, "/api/traffic/ingest", nil)
	preflight.Header.Set("Origin", "chrome-extension://fixture")
	preflight.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, preflight)
	if w.Code != http.StatusNoContent || !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("extension preflight: %d, headers=%v", w.Code, w.Header())
	}

	body := `{"exchanges":[{"method":"POST","url":"https://lab.example.test/login","status":200,"requestHeaders":{"Content-Type":"application/json"},"requestBody":"{\"user\":\"fixture\"}","responseHeaders":{"Content-Type":"application/json"},"responseBody":"{\"ok\":true}"},{"method":"GET","url":"file:///etc/passwd","status":200}]}`
	request := func(token string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/api/traffic/ingest", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		out := httptest.NewRecorder()
		h.ServeHTTP(out, r)
		return out
	}
	if got := request(""); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated capture: %d %s", got.Code, got.Body.String())
	}
	token, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	if got := request(token); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"ingested":1`) || !strings.Contains(got.Body.String(), `"failed":1`) {
		t.Fatalf("capture result: %d %s", got.Code, got.Body.String())
	}
	rows, total, err := tr.Page(traffic.PageQuery{Query: "login", RespMin: -1, RespMax: -1}, 0, 10)
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("captured exchange missing from normal search: total=%d rows=%d err=%v", total, len(rows), err)
	}
}
