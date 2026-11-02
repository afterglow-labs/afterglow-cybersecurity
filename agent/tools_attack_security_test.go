package agent

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type rejectNetworkRoundTripper struct{ called bool }

func (r *rejectNetworkRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	r.called = true
	return nil, nil
}

func TestReplayFailsClosedWithoutCaptureAndScope(t *testing.T) {
	tools := NewToolSet(nil, "worker")
	if err := tools.replayReady("https://example.com/item/1"); err == nil || !strings.Contains(err.Error(), "operator-entered") {
		t.Fatalf("missing task scope was accepted: %v", err)
	}
	if _, err := tools.proxyClient(); err == nil || !strings.Contains(err.Error(), "recording proxy") {
		t.Fatalf("missing capture proxy was accepted: %v", err)
	}
	if session, err := tools.newReplaySession("https://example.com/item/1"); err == nil || session != nil {
		t.Fatalf("request session escaped the fail-closed path: %v", err)
	}
}

func TestReplayIdentityTemplateEscapesCandidateValues(t *testing.T) {
	candidate := `x"},"admin":true,"other":"y & / ?`
	for _, test := range []struct {
		url, body, current, position string
	}{
		{"https://example.com/items/123", "", "123", "path"},
		{"https://example.com/items?id=123", "", "123", "query"},
		{"https://example.com/items", `{"id":"123","admin":false}`, "123", "body_json"},
		{"https://example.com/items", "id=123&admin=false", "123", "body_form"},
	} {
		urlTemplate, bodyTemplate, position, ok := swapIDTemplate(test.url, test.body, test.current)
		if !ok || position != test.position {
			t.Fatalf("template %q: position=%q ok=%v", test.url, position, ok)
		}
		target, body := fillIDTemplate(urlTemplate, bodyTemplate, position, candidate)
		switch position {
		case "path":
			u, _ := url.Parse(target)
			if !strings.HasSuffix(u.Path, "/"+candidate) {
				t.Fatalf("candidate escaped path value: %q", target)
			}
		case "query":
			u, _ := url.Parse(target)
			if u.Query().Get("id") != candidate || len(u.Query()) != 1 {
				t.Fatalf("candidate escaped query value: %q", target)
			}
		case "body_json":
			var doc map[string]any
			if json.Unmarshal([]byte(body), &doc) != nil || doc["id"] != candidate || doc["admin"] != false || len(doc) != 2 {
				t.Fatalf("candidate escaped JSON value: %q", body)
			}
		case "body_form":
			form, err := url.ParseQuery(body)
			if err != nil || form.Get("id") != candidate || form.Get("admin") != "false" || len(form) != 2 {
				t.Fatalf("candidate escaped form value: %q", body)
			}
		}
	}
}

func TestReplayGuestDropsCredentialHeaders(t *testing.T) {
	h := http.Header{"Authorization": {"Bearer secret"}, "X-Api-Key": {"secret"}, "Cookie": {"session=secret"}, "Accept": {"application/json"}}
	stripIdentityHeaders(h)
	if len(h) != 1 || h.Get("Accept") != "application/json" {
		t.Fatalf("guest retained identity headers: %v", h)
	}
}

func TestReplaySessionRejectsUnapprovedTargetAndMethodBeforeNetwork(t *testing.T) {
	transport := &rejectNetworkRoundTripper{}
	session := &replaySession{client: &http.Client{Transport: transport}, targets: map[string]bool{"https://example.com/allowed": true}}
	for _, test := range []struct{ method, target string }{
		{"GET", "https://example.com/other"},
		{"DELETE", "https://example.com/allowed"},
	} {
		if got := session.fireHTTP(t.Context(), test.method, test.target, nil, nil); got.Err == "" || got.Status != 0 {
			t.Fatalf("unapproved replay was accepted: %+v", got)
		}
	}
	if transport.called {
		t.Fatal("blocked replay reached the network")
	}
}
