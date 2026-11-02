package agent

// Native, task-scoped request replay tools.
//
// Request-replay tools the worker fires without leaving the harness (no Bash,
// no Python dependency): race_probe (one concurrent burst of conflicting
// variants + committed-state diff), session_store / identity_switch /
// identity_matrix (named cookie identities and the actor x id matrix), and
// inflight_modify (price/amount tamper variants of a JSON request).
//
// Every request requires an active recording proxy and an operator-entered
// target scope. No direct egress or redirect fallback is permitted. These are
// screening tools; their results do not by themselves establish a finding.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Autumn-27/artex/db"
	actool "github.com/Autumn-27/norma/tool"
)

// --- shared plumbing ---------------------------------------------------------

// attackIdentity is one named actor the replay tools can switch to. The roster
// lives on the ToolSet: one per run (per intent), in-memory, lost with the run.
type attackIdentity struct {
	Name    string
	Cookie  string
	Headers []string // "K: V"
	Note    string
}

// SetProxy points the attack tools' HTTP client at the recording proxy (the
// MITM capture when capture is on) plus its CA cert so re-signed HTTPS
// verifies. An empty address or CA disables replay. Mirrors Worker.SetProxy.
func (t *ToolSet) SetProxy(addr, caCert string) {
	t.proxyAddr = addr
	t.proxyCACert = caCert
}

// proxyClient fails closed when capture is unavailable or misconfigured.
func (t *ToolSet) proxyClient() (*http.Client, error) {
	if t.proxyAddr == "" || t.proxyCACert == "" {
		return nil, fmt.Errorf("recording proxy is required for native replay")
	}
	u, err := url.Parse(t.proxyAddr)
	if err != nil || u.Scheme != "http" || u.Hostname() == "" || u.User != nil {
		return nil, fmt.Errorf("recording proxy address is invalid")
	}
	tr := &http.Transport{
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     30 * time.Second,
		ForceAttemptHTTP2:   true,
		Proxy:               http.ProxyURL(u),
		DisableKeepAlives:   true,
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	pem, err := os.ReadFile(t.proxyCACert)
	if err != nil || !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("recording proxy CA is unavailable")
	}
	tr.TLSClientConfig.RootCAs = pool
	return &http.Client{Transport: tr, Timeout: 20 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

type fireResult struct {
	Status int
	MS     int64
	Body   []byte
	Err    string
}

type replaySession struct {
	client  *http.Client
	targets map[string]bool
}

func (t *ToolSet) replayReady(target string) error {
	if len(target) > 4096 {
		return fmt.Errorf("target URL exceeds replay limit")
	}
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.Fragment != "" {
		return fmt.Errorf("target URL is invalid")
	}
	if _, ok := t.targetInOperatorScope(u.Hostname()); !ok {
		return fmt.Errorf("target is outside operator-entered task scope")
	}
	globalRules, err := t.as.ListAssetInterceptRules()
	if err != nil {
		return fmt.Errorf("target interception rules are unavailable")
	}
	taskBlocks, taskAllows, err := t.as.TaskInterceptRulesSplit(t.taskID)
	if err != nil {
		return fmt.Errorf("task interception rules are unavailable")
	}
	domains, ips := []string{}, []string{}
	if net.ParseIP(u.Hostname()) != nil {
		ips = append(ips, u.Hostname())
	} else {
		domains = append(domains, u.Hostname())
	}
	if !db.EvaluateAssetGate(append(globalRules, taskBlocks...), taskAllows, domains, ips, []string{target}).Allowed {
		return fmt.Errorf("target is blocked by interception rules")
	}
	return nil
}

// newReplaySession checks every distinct URL before the first request. A burst
// then reuses one client and one immutable list of approved destinations.
func (t *ToolSet) newReplaySession(targets ...string) (*replaySession, error) {
	approved := make(map[string]bool, len(targets))
	for _, target := range targets {
		if target == "" || approved[target] {
			continue
		}
		if err := t.replayReady(target); err != nil {
			return nil, err
		}
		approved[target] = true
	}
	if len(approved) == 0 {
		return nil, fmt.Errorf("at least one replay target is required")
	}
	client, err := t.proxyClient()
	if err != nil {
		return nil, err
	}
	return &replaySession{client: client, targets: approved}, nil
}

// fireHTTP sends only to a URL admitted by newReplaySession. The preflight is
// outside the burst so the requests can leave the proxy together.
func (s *replaySession) fireHTTP(ctx context.Context, method, target string, hdrs http.Header, body []byte) fireResult {
	if s == nil || !s.targets[target] {
		return fireResult{Err: "target was not approved for this replay"}
	}
	if len(body) > 64<<10 {
		return fireResult{Err: "request exceeds replay limits"}
	}
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "OPTIONS":
	default:
		return fireResult{Err: "request method is not allowed for native replay"}
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return fireResult{Err: "request method or URL is invalid"}
	}
	headerBytes := 0
	for k, vs := range hdrs {
		if forbiddenReplayHeader(k) || len(vs) > 8 || len(k) == 0 || len(k) > 80 || strings.ContainsAny(k, "\r\n\x00 :") {
			return fireResult{Err: "request includes a forbidden header"}
		}
		for _, v := range vs {
			headerBytes += len(k) + len(v)
			if headerBytes > 8192 || strings.ContainsAny(v, "\r\n\x00") {
				return fireResult{Err: "request headers exceed replay limits"}
			}
			req.Header.Add(k, v)
		}
	}
	t0 := time.Now()
	resp, err := s.client.Do(req)
	if err != nil {
		return fireResult{MS: time.Since(t0).Milliseconds(), Err: "request through recording proxy failed"}
	}
	defer resp.Body.Close()
	raw, rerr := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	f := fireResult{Status: resp.StatusCode, MS: time.Since(t0).Milliseconds(), Body: raw}
	if rerr != nil {
		f.Err = "response read failed"
	}
	return f
}

func forbiddenReplayHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Host", "Proxy-Authorization", "Proxy-Authenticate", "Connection", "Proxy-Connection", "Transfer-Encoding", "Content-Length", "Upgrade", "Forwarded", "X-Forwarded-Host", "X-Original-Url", "X-Rewrite-Url":
		return true
	}
	return false
}

// headerPairs parses "K: V" lines into a Header, skipping malformed entries.
func headerPairs(pairs []string) http.Header {
	h := http.Header{}
	for _, p := range pairs {
		if k, v, ok := strings.Cut(p, ":"); ok {
			if k = strings.TrimSpace(k); k != "" {
				h.Add(k, strings.TrimSpace(v))
			}
		}
	}
	return h
}

func stripIdentityHeaders(h http.Header) {
	for k := range h {
		name := strings.ToLower(k)
		if strings.Contains(name, "cookie") || strings.Contains(name, "auth") || strings.Contains(name, "token") || strings.Contains(name, "key") || strings.Contains(name, "session") {
			h.Del(k)
		}
	}
}

func cloneHeader(h http.Header) http.Header {
	out := http.Header{}
	for k, vs := range h {
		out[k] = append([]string(nil), vs...)
	}
	return out
}

func clipText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("... [%d bytes total]", len(s))
}

func bodyDigest(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// fireEntry is the per-request row returned by the replay tools.
func fireEntry(label string, f fireResult, snippetSpan int) map[string]any {
	m := map[string]any{
		"label":  label,
		"status": f.Status,
		"ms":     f.MS,
		"bytes":  len(f.Body),
	}
	if snippetSpan > 0 && len(f.Body) > 0 {
		m["snippet"] = clipText(string(f.Body), snippetSpan)
	}
	if f.Err != "" {
		m["error"] = f.Err
	}
	return m
}

func verdictByAccepted(n, total int) string {
	switch {
	case n == 0:
		return "NONE_ACCEPTED"
	case n == total:
		return "ALL_ACCEPTED"
	default:
		return "PARTIAL_ACCEPTED"
	}
}

func strContains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// --- json diff (committed-state comparison) ----------------------------------

// normalizeNum collapses 100.0 to 100 so numeric-vs-integer encoding does not
// register as a state change.
func normalizeNum(v any) any {
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		if f > -1e15 && f < 1e15 {
			return int64(f)
		}
	}
	return v
}

func jsonEq(a, b any) bool {
	ja, _ := json.Marshal(normalizeNum(a))
	jb, _ := json.Marshal(normalizeNum(b))
	return string(ja) == string(jb)
}

// jsonDiffPaths collects dot-paths whose values differ between two parsed
// JSON values. Capped (40 paths, depth 8) so a chatty state read cannot flood
// the tool output.
func jsonDiffPaths(a, b any, prefix string, depth int, paths *[]string) {
	if len(*paths) >= 40 || depth > 8 {
		return
	}
	am, aOK := a.(map[string]any)
	bm, bOK := b.(map[string]any)
	if aOK && bOK {
		keys := map[string]bool{}
		for k := range am {
			keys[k] = true
		}
		for k := range bm {
			keys[k] = true
		}
		for k := range keys {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			av, aHas := am[k]
			bv, bHas := bm[k]
			switch {
			case !aHas:
				*paths = append(*paths, p+" (added)")
			case !bHas:
				*paths = append(*paths, p+" (removed)")
			default:
				switch {
				case sameContainer(av, bv):
					jsonDiffPaths(av, bv, p, depth+1, paths)
				case !jsonEq(av, bv):
					*paths = append(*paths, p+" (changed)")
				}
			}
		}
		return
	}
	aa, aOK := a.([]any)
	bb, bOK := b.([]any)
	if aOK && bOK {
		if len(aa) != len(bb) {
			*paths = append(*paths, fmt.Sprintf("%s (array length %d -> %d)", prefix, len(aa), len(bb)))
			return
		}
		for i := range aa {
			p := fmt.Sprintf("%s[%d]", prefix, i)
			switch {
			case sameContainer(aa[i], bb[i]):
				jsonDiffPaths(aa[i], bb[i], p, depth+1, paths)
			case !jsonEq(aa[i], bb[i]):
				*paths = append(*paths, p+" (changed)")
			}
		}
		return
	}
}

// sameContainer reports whether both values are the same container kind
// (map/map or array/array) so the diff recurses instead of flattening.
func sameContainer(a, b any) bool {
	_, aMap := a.(map[string]any)
	_, bMap := b.(map[string]any)
	if aMap || bMap {
		return aMap && bMap
	}
	_, aArr := a.([]any)
	_, bArr := b.([]any)
	return aArr && bArr
}

// --- id swapping (identity_matrix) -------------------------------------------

const idSentinel = "%__ARTEX_ID__%"

// idSentinelURL is the sentinel after url.URL / url.Values encoding — the URL
// and form-body branches carry this form, the JSON body branch carries the raw
// one. Re-filling must match the form that is actually present.
var idSentinelURL = url.PathEscape(idSentinel)

func idEq(val any, current string) bool {
	switch s := val.(type) {
	case string:
		return s == current
	case float64:
		if f, err := strconv.ParseFloat(current, 64); err == nil {
			return s == f
		}
	}
	return false
}

// replaceIDAny sets the first value equal to current to the sentinel and
// reports whether one was found. Maps are walked in sorted-key order so the
// first hit is deterministic.
func replaceIDAny(v any, current string) bool {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if idEq(t[k], current) {
				t[k] = idSentinel
				return true
			}
		}
		for _, k := range keys {
			if replaceIDAny(t[k], current) {
				return true
			}
		}
	case []any:
		for i := range t {
			if idEq(t[i], current) {
				t[i] = idSentinel
				return true
			}
			if replaceIDAny(t[i], current) {
				return true
			}
		}
	}
	return false
}

// swapIDTemplate locates the concrete id in the request (path segment from the
// tail, query value, JSON body value, or form value) and returns the request
// with a sentinel in its place, plus the position label. The caller re-fills
// the sentinel per candidate id.
func swapIDTemplate(target, body, current string) (string, string, string, bool) {
	if u, err := url.Parse(target); err == nil {
		parts := strings.Split(u.Path, "/")
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] == current {
				parts[i] = idSentinel
				nu := *u
				nu.Path = strings.Join(parts, "/")
				return nu.String(), body, "path", true
			}
		}
		q := u.Query()
		hit := false
		for k, vs := range q {
			for i, v := range vs {
				if v == current {
					vs[i] = idSentinel
					q[k] = vs
					hit = true
				}
			}
		}
		if hit {
			nu := *u
			nu.RawQuery = q.Encode()
			return nu.String(), body, "query", true
		}
	}
	if body != "" {
		trimmed := strings.TrimSpace(body)
		if strings.HasPrefix(trimmed, "{") {
			var v any
			if json.Unmarshal([]byte(body), &v) == nil && replaceIDAny(v, current) {
				if b, err := json.Marshal(v); err == nil {
					return target, string(b), "body_json", true
				}
			}
		} else if strings.Contains(body, "=") {
			vals, err := url.ParseQuery(body)
			if err == nil {
				hit := false
				for k, vs := range vals {
					for i, v := range vs {
						if v == current {
							vs[i] = idSentinel
							vals[k] = vs
							hit = true
						}
					}
				}
				if hit {
					return target, vals.Encode(), "body_form", true
				}
			}
		}
	}
	return target, body, "", false
}

// fillIDTemplate keeps candidate IDs inside their original URL or body value.
// Plain string replacement in JSON would let an ID containing quotes change
// the request structure; form values need query escaping as well.
func fillIDTemplate(target, body, position, id string) (string, string) {
	switch position {
	case "path":
		return strings.ReplaceAll(target, idSentinelURL, url.PathEscape(id)), body
	case "query":
		return strings.ReplaceAll(target, idSentinelURL, url.QueryEscape(id)), body
	case "body_form":
		return target, strings.ReplaceAll(body, idSentinelURL, url.QueryEscape(id))
	case "body_json":
		quoted, _ := json.Marshal(id)
		return target, strings.ReplaceAll(body, strconv.Quote(idSentinel), string(quoted))
	}
	return target, body
}

// --- race_probe ----------------------------------------------------------------

// raceProbe fires N conflicting variants of one request in a single concurrent
// burst, then reads the committed state before/after. The classic single-use
// resource race (withdraw, coupon, credit, checkout) shows up as: every variant
// accepted while the state changed less than N times.
func (t *ToolSet) raceProbe() actool.CoreTool {
	return t.writeExpTool("race_probe",
		"Fire N conflicting request variants in ONE concurrent burst, then read the committed state before and after — race-condition screening on single-use resources (withdrawal, coupon, credit, checkout, one-shot token). Every request is captured as evidence. Screening only: judge the result against the state diff — fewer state changes than accepted variants is a double-execution candidate; confirm with a read-back before reporting.",
		obj(map[string]any{
			"url":                str("target URL, e.g. http://host/api/withdraw"),
			"method":             str("HTTP method (default: POST when a variant is present, else GET)"),
			"body":               str("optional base body; ignored when body_variants are given"),
			"body_variants":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "conflicting payloads fired in the burst, round-robin (e.g. the same withdrawal body, or two different coupon codes)"},
			"content_type":       str("default application/json when a body is present"),
			"headers":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "extra headers as \"K: V\" lines (e.g. the session cookie)"},
			"burst":              intp("concurrent requests to fire, 1-16 (default 8)"),
			"state_read_url":     str("optional: URL that reads the committed state (e.g. GET /api/me) — read before and after the burst"),
			"state_read_method":  str("method for the state read (default GET)"),
			"state_read_headers": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "headers for the state read, \"K: V\" lines"},
			"state_read_body":    str("optional body for the state read"),
		}, "url", "body_variants"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				URL              string   `json:"url"`
				Method           string   `json:"method"`
				Body             string   `json:"body"`
				BodyVariants     []string `json:"body_variants"`
				ContentType      string   `json:"content_type"`
				Headers          []string `json:"headers"`
				Burst            int      `json:"burst"`
				StateReadURL     string   `json:"state_read_url"`
				StateReadMethod  string   `json:"state_read_method"`
				StateReadHeaders []string `json:"state_read_headers"`
				StateReadBody    string   `json:"state_read_body"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf("race_probe: bad input JSON: " + err.Error()), nil
			}
			a.URL = strings.TrimSpace(a.URL)
			if a.URL == "" {
				return actool.Errorf("race_probe: url is required"), nil
			}
			if a.StateReadURL != "" {
				if a.StateReadMethod != "" && !strings.EqualFold(a.StateReadMethod, "GET") && !strings.EqualFold(a.StateReadMethod, "HEAD") {
					return actool.Errorf("race_probe state read must use GET or HEAD"), nil
				}
			}
			variants := make([]string, 0, len(a.BodyVariants))
			for _, v := range a.BodyVariants {
				if strings.TrimSpace(v) != "" {
					if len(v) > 64<<10 || len(variants) >= 8 {
						return actool.Errorf("race_probe: at most 8 variants of 64 KiB each"), nil
					}
					variants = append(variants, v)
				}
			}
			if len(variants) == 0 && strings.TrimSpace(a.Body) != "" {
				variants = []string{a.Body}
			}
			if len(variants) == 0 {
				return actool.Errorf("race_probe: body_variants is required (the conflicting payloads to race)"), nil
			}
			if a.Burst <= 0 {
				a.Burst = 8
			}
			if a.Burst > 16 {
				return actool.Errorf("race_probe: burst exceeds 16 requests"), nil
			}
			if a.Method == "" {
				if len(variants) > 0 {
					a.Method = "POST"
				} else {
					a.Method = "GET"
				}
			}
			hdrs := headerPairs(a.Headers)
			if len(variants) > 0 && hdrs.Get("Content-Type") == "" {
				if a.ContentType != "" {
					hdrs.Set("Content-Type", a.ContentType)
				} else {
					hdrs.Set("Content-Type", "application/json")
				}
			}
			stateHdrs := headerPairs(a.StateReadHeaders)
			if a.StateReadBody != "" && stateHdrs.Get("Content-Type") == "" {
				stateHdrs.Set("Content-Type", "application/json")
			}
			session, err := t.newReplaySession(a.URL, a.StateReadURL)
			if err != nil {
				return actool.Errorf("race_probe: " + err.Error()), nil
			}
			ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()

			var stateBefore fireResult
			if a.StateReadURL != "" {
				stateBefore = session.fireHTTP(ctx, a.StateReadMethod, a.StateReadURL, stateHdrs, []byte(a.StateReadBody))
			}

			res := make([]map[string]any, a.Burst)
			var wg sync.WaitGroup
			var ready sync.WaitGroup
			ready.Add(a.Burst)
			start := make(chan struct{})
			for i := 0; i < a.Burst; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					ready.Done()
					<-start
					v := variants[i%len(variants)]
					f := session.fireHTTP(ctx, a.Method, a.URL, hdrs, []byte(v))
					m := fireEntry(fmt.Sprintf("v%d (variant %d)", i, i%len(variants)), f, 200)
					m["sent"] = clipText(v, 200)
					res[i] = m
				}(i)
			}
			ready.Wait()
			close(start)
			wg.Wait()

			n2xx := 0
			for _, r := range res {
				if s, _ := r["status"].(int); s >= 200 && s < 300 {
					n2xx++
				}
			}

			out := map[string]any{
				"url":        a.URL,
				"method":     a.Method,
				"burst":      a.Burst,
				"n_variants": len(variants),
				"n_accepted": n2xx,
				"results":    res,
				"verdict":    verdictByAccepted(n2xx, a.Burst),
				"note":       "screening only: with a state read, compare how many times the state changed against n_accepted — fewer changes than accepted variants = double-execution candidate; confirm with a read-back before report_finding",
			}
			if a.StateReadURL != "" {
				stateAfter := session.fireHTTP(ctx, a.StateReadMethod, a.StateReadURL, stateHdrs, []byte(a.StateReadBody))
				out["state_before"] = clipText(string(stateBefore.Body), 800)
				out["state_after"] = clipText(string(stateAfter.Body), 800)
				var sb, sa any
				if json.Unmarshal(stateBefore.Body, &sb) == nil && json.Unmarshal(stateAfter.Body, &sa) == nil {
					var paths []string
					jsonDiffPaths(sb, sa, "", 0, &paths)
					if len(paths) == 0 {
						out["state_diff"] = "no changes"
					} else {
						out["state_diff"] = paths
					}
				} else {
					out["state_diff"] = "state bodies are not JSON — compare the snippets above"
				}
			}
			return jsonResult(out)
		})
}

// --- session_store / identity_switch / identity_matrix -------------------------

func (t *ToolSet) putIdentity(id attackIdentity) bool {
	t.identMu.Lock()
	defer t.identMu.Unlock()
	if t.identities == nil {
		t.identities = map[string]attackIdentity{}
	}
	if len(t.identities) >= 8 && t.identities[strings.ToLower(id.Name)].Name == "" {
		return false
	}
	t.identities[strings.ToLower(id.Name)] = id
	return true
}

func (t *ToolSet) lookupIdentity(name string) (attackIdentity, bool) {
	t.identMu.Lock()
	defer t.identMu.Unlock()
	id, ok := t.identities[strings.ToLower(name)]
	return id, ok
}

func (t *ToolSet) identityRoster() []map[string]any {
	t.identMu.Lock()
	defer t.identMu.Unlock()
	out := make([]map[string]any, 0, len(t.identities))
	for _, id := range t.identities {
		out = append(out, map[string]any{
			"name":      id.Name,
			"cookie":    maskSecret(id.Cookie),
			"n_headers": len(id.Headers),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["name"].(string) < out[j]["name"].(string) })
	return out
}

func rosterNames(roster []map[string]any) string {
	if len(roster) == 0 {
		return "(none yet)"
	}
	names := make([]string, 0, len(roster))
	for _, r := range roster {
		if n, ok := r["name"].(string); ok {
			names = append(names, n)
		}
	}
	return strings.Join(names, ", ")
}

func maskSecret(s string) string {
	if s == "" {
		return "(none)"
	}
	return "(stored)"
}

// sessionStore registers a named identity (cookie + optional headers) for this
// run so the replay tools can act as different actors.
func (t *ToolSet) sessionStore() actool.CoreTool {
	return t.writeExpTool("session_store",
		"Register a named identity (cookie + optional headers) for this run, so identity_switch / identity_matrix can replay requests as different actors (alice, bob, admin...) or as guest. The roster is in-memory and lives only for the current run. Store every actor you hold a session for before replaying.",
		obj(map[string]any{
			"name":    str("identity name, e.g. alice / bob / admin / service-key"),
			"cookie":  str("the full Cookie header value for this actor (may be empty for token-based actors)"),
			"headers": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "extra identity headers as \"K: V\" lines (e.g. \"Authorization: Bearer ...\")"},
			"note":    str("optional: how this session was obtained (login traffic id, token source)"),
		}, "name"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				Name    string   `json:"name"`
				Cookie  string   `json:"cookie"`
				Headers []string `json:"headers"`
				Note    string   `json:"note"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf("session_store: bad input JSON: " + err.Error()), nil
			}
			a.Name = strings.TrimSpace(a.Name)
			if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,31}$`).MatchString(a.Name) || len(a.Cookie) > 8192 || strings.ContainsAny(a.Cookie, "\r\n\x00") || len(a.Headers) > 16 || len(a.Note) > 256 {
				return actool.Errorf("session_store: invalid identity name or credential size"), nil
			}
			for _, pair := range a.Headers {
				key, val, ok := strings.Cut(pair, ":")
				key = strings.TrimSpace(key)
				if !ok || key == "" || len(key) > 80 || len(val) > 8192 || forbiddenReplayHeader(key) || strings.ContainsAny(key+val, "\r\n\x00") {
					return actool.Errorf("session_store: invalid identity header"), nil
				}
			}
			if !t.putIdentity(attackIdentity{Name: a.Name, Cookie: a.Cookie, Headers: a.Headers, Note: a.Note}) {
				return actool.Errorf("session_store: at most 8 identities per run"), nil
			}
			return jsonResult(map[string]any{
				"stored": a.Name,
				"roster": t.identityRoster(),
				"note":   "roster lives for this run only — re-register after a new intent",
			})
		})
}

// identitySwitch replays one request as a stored identity (or bare, with no
// identity headers at all).
func (t *ToolSet) identitySwitch() actool.CoreTool {
	return t.writeExpTool("identity_switch",
		"Replay one request as a stored identity (or bare, with no identity headers) — the A/B half of BOLA/BFLA testing: same request, different actor, diff the responses. Use session_store first to register the actor. The response digest lets you compare equality across calls.",
		obj(map[string]any{
			"url":     str("request URL"),
			"method":  str("HTTP method (default GET)"),
			"body":    str("optional request body (replayed verbatim)"),
			"headers": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "request headers as \"K: V\" lines (the identity's headers/cookie are merged on top)"},
			"as":      str("identity name from session_store; omit to replay bare (no cookie, no auth)"),
		}, "url"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				URL     string   `json:"url"`
				Method  string   `json:"method"`
				Body    string   `json:"body"`
				Headers []string `json:"headers"`
				As      string   `json:"as"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf("identity_switch: bad input JSON: " + err.Error()), nil
			}
			if strings.TrimSpace(a.URL) == "" {
				return actool.Errorf("identity_switch: url is required"), nil
			}
			session, err := t.newReplaySession(a.URL)
			if err != nil {
				return actool.Errorf("identity_switch: " + err.Error()), nil
			}
			ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			hdrs := headerPairs(a.Headers)
			stripIdentityHeaders(hdrs)
			who := "bare"
			if name := strings.TrimSpace(a.As); name != "" {
				id, ok := t.lookupIdentity(name)
				if !ok {
					return actool.Errorf("identity_switch: identity " + name + " is not stored — session_store it first (known: " + rosterNames(t.identityRoster()) + ")"), nil
				}
				for _, p := range id.Headers {
					if k, v, ok := strings.Cut(p, ":"); ok {
						hdrs.Set(strings.TrimSpace(k), strings.TrimSpace(v))
					}
				}
				if id.Cookie != "" {
					hdrs.Set("Cookie", id.Cookie)
				}
				who = name
			}
			if a.Body != "" && hdrs.Get("Content-Type") == "" {
				hdrs.Set("Content-Type", "application/json")
			}
			f := session.fireHTTP(ctx, a.Method, a.URL, hdrs, []byte(a.Body))
			return jsonResult(map[string]any{
				"url":      a.URL,
				"method":   a.Method,
				"identity": who,
				"status":   f.Status,
				"ms":       f.MS,
				"bytes":    len(f.Body),
				"digest":   bodyDigest(f.Body),
				"snippet":  clipText(string(f.Body), 1000),
				"error":    f.Err,
			})
		})
}

type matrixActor struct {
	name  string
	ident attackIdentity
	guest bool
}

// identityMatrix runs the full actor x id matrix: each stored identity (and
// optionally guest) replays the request with each candidate object id swapped
// in. The concrete id must literally appear in the url/query/body.
func (t *ToolSet) identityMatrix() actool.CoreTool {
	return t.writeExpTool("identity_matrix",
		"Full actor x object-id matrix in one call: replay one request as each named identity (session_store; include \"guest\" for a no-cookie actor) with each candidate id swapped into the request. The id must literally appear in the url path, a query value, or the JSON/form body — pass the exact value currently in the request. Identical responses across different actors (same digest) are the BOLA signature: SHARED_RESPONSE_CANDIDATE. Screening only — leak judgment is yours from the snippets.",
		obj(map[string]any{
			"url":     str("request URL containing the concrete id"),
			"method":  str("HTTP method (default GET)"),
			"body":    str("optional request body containing the id"),
			"headers": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "base request headers as \"K: V\" lines"},
			"id":      str("the concrete id value currently in the request, e.g. 101"),
			"ids":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "candidate object ids, 1-8 — include your own and another actor's id"},
			"actors":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "identity names from session_store, 1-4; \"guest\" = no cookie/auth"},
		}, "url", "id", "ids", "actors"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				URL     string   `json:"url"`
				Method  string   `json:"method"`
				Body    string   `json:"body"`
				Headers []string `json:"headers"`
				ID      string   `json:"id"`
				IDs     []string `json:"ids"`
				Actors  []string `json:"actors"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf("identity_matrix: bad input JSON: " + err.Error()), nil
			}
			if strings.TrimSpace(a.URL) == "" || strings.TrimSpace(a.ID) == "" {
				return actool.Errorf("identity_matrix: url and id are required (id = the concrete value currently in the request)"), nil
			}
			if err := t.replayReady(a.URL); err != nil {
				return actool.Errorf("identity_matrix: " + err.Error()), nil
			}
			if len(a.ID) > 256 {
				return actool.Errorf("identity_matrix: id exceeds 256 bytes"), nil
			}
			ids := make([]string, 0, len(a.IDs))
			for _, v := range a.IDs {
				if v = strings.TrimSpace(v); v != "" && !strContains(ids, v) {
					ids = append(ids, v)
				}
			}
			actors := make([]string, 0, len(a.Actors))
			for _, v := range a.Actors {
				if v = strings.TrimSpace(v); v != "" && !strContains(actors, v) {
					actors = append(actors, v)
				}
			}
			if len(ids) == 0 || len(actors) == 0 {
				return actool.Errorf("identity_matrix: ids and actors are required"), nil
			}
			if len(ids) > 8 {
				return actool.Errorf("identity_matrix: at most 8 ids"), nil
			}
			if len(actors) > 4 {
				return actool.Errorf("identity_matrix: at most 4 actors"), nil
			}
			for _, id := range ids {
				if len(id) > 256 {
					return actool.Errorf("identity_matrix: id exceeds 256 bytes"), nil
				}
			}
			resolved := make([]matrixActor, 0, len(actors))
			for _, name := range actors {
				if strings.EqualFold(name, "guest") {
					resolved = append(resolved, matrixActor{name: "guest", guest: true})
					continue
				}
				id, ok := t.lookupIdentity(name)
				if !ok {
					return actool.Errorf("identity_matrix: actor " + name + " is not stored — session_store it first (known: " + rosterNames(t.identityRoster()) + ")"), nil
				}
				resolved = append(resolved, matrixActor{name: name, ident: id})
			}
			turl, tbody, position, ok := swapIDTemplate(a.URL, a.Body, a.ID)
			if !ok {
				return actool.Errorf("identity_matrix: id " + a.ID + " not found in the url path, a query value, or the body — pass the exact value currently in the request"), nil
			}
			allowedURLs := make([]string, 0, len(ids))
			for _, id := range ids {
				u, _ := fillIDTemplate(turl, tbody, position, id)
				allowedURLs = append(allowedURLs, u)
			}
			session, err := t.newReplaySession(allowedURLs...)
			if err != nil {
				return actool.Errorf("identity_matrix: " + err.Error()), nil
			}
			ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			baseHdrs := headerPairs(a.Headers)
			stripIdentityHeaders(baseHdrs)
			if a.Body != "" && baseHdrs.Get("Content-Type") == "" {
				if position == "body_form" {
					baseHdrs.Set("Content-Type", "application/x-www-form-urlencoded")
				} else {
					baseHdrs.Set("Content-Type", "application/json")
				}
			}

			type cellJob struct {
				actor string
				ident attackIdentity
				guest bool
				id    string
			}
			jobs := make([]cellJob, 0, len(resolved)*len(ids))
			for _, r := range resolved {
				for _, id := range ids {
					jobs = append(jobs, cellJob{actor: r.name, ident: r.ident, guest: r.guest, id: id})
				}
			}
			results := make([]map[string]any, len(jobs))
			var wg sync.WaitGroup
			sem := make(chan struct{}, 6)
			for i, j := range jobs {
				wg.Add(1)
				go func(i int, j cellJob) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					u, b := fillIDTemplate(turl, tbody, position, j.id)
					hdrs := cloneHeader(baseHdrs)
					if !j.guest {
						for _, p := range j.ident.Headers {
							if k, v, ok := strings.Cut(p, ":"); ok {
								hdrs.Set(strings.TrimSpace(k), strings.TrimSpace(v))
							}
						}
						if j.ident.Cookie != "" {
							hdrs.Set("Cookie", j.ident.Cookie)
						}
					}
					f := session.fireHTTP(ctx, a.Method, u, hdrs, []byte(b))
					m := fireEntry(j.actor+" -> "+j.id, f, 160)
					m["digest"] = bodyDigest(f.Body)
					results[i] = m
				}(i, j)
			}
			wg.Wait()

			byDigest := map[string]map[string]bool{}
			for _, c := range results {
				d, _ := c["digest"].(string)
				if d == "" {
					continue
				}
				if byDigest[d] == nil {
					byDigest[d] = map[string]bool{}
				}
				if label, ok := c["label"].(string); ok {
					if actor, _, ok := strings.Cut(label, " -> "); ok {
						byDigest[d][actor] = true
					}
				}
			}
			shared := []map[string]any{}
			for d, actorSet := range byDigest {
				if len(actorSet) > 1 {
					names := make([]string, 0, len(actorSet))
					for n := range actorSet {
						names = append(names, n)
					}
					sort.Strings(names)
					shared = append(shared, map[string]any{
						"digest": d,
						"actors": names,
					})
				}
			}
			sort.Slice(shared, func(i, j int) bool { return shared[i]["digest"].(string) < shared[j]["digest"].(string) })
			n2xx := 0
			for _, c := range results {
				if s, _ := c["status"].(int); s >= 200 && s < 300 {
					n2xx++
				}
			}
			verdict := "MATRIX_COMPLETE"
			if len(shared) > 0 {
				verdict = "SHARED_RESPONSE_CANDIDATE"
			}
			return jsonResult(map[string]any{
				"url_template":           strings.ReplaceAll(turl, idSentinelURL, "<ID>"),
				"body_template":          strings.ReplaceAll(strings.ReplaceAll(tbody, idSentinelURL, "<ID>"), idSentinel, "<ID>"),
				"id_position":            position,
				"actors":                 actors,
				"ids":                    ids,
				"n_cells":                len(results),
				"n_2xx":                  n2xx,
				"verdict":                verdict,
				"shared_response_groups": shared,
				"cells":                  results,
				"note":                   "screening only: a shared response across actors with real data in the snippet = BOLA candidate — report_finding with the cell pair as evidence",
			})
		})
}

// --- inflight_modify -----------------------------------------------------------

var moneyKeyRe = regexp.MustCompile(`(?i)^(price|amount|total|subtotal|grand_total|qty|quantity|count|discount|fee|cost|balance|payment|charge)$`)

var tamperModes = []string{"zero", "cent", "negative", "one", "huge", "null"}

func tamperValue(orig any, mode string) any {
	_, isStr := orig.(string)
	switch mode {
	case "zero":
		if isStr {
			return "0"
		}
		return 0
	case "cent":
		if isStr {
			return "0.01"
		}
		return 0.01
	case "negative":
		if isStr {
			return "-1"
		}
		return -1
	case "one":
		if isStr {
			return "1"
		}
		return 1
	case "huge":
		if isStr {
			return "999999999"
		}
		return 999999999
	case "null":
		return nil
	}
	return nil
}

// findKeyAny reports whether key exists anywhere in the parsed JSON (sorted
// walk → deterministic first hit).
func findKeyAny(v any, key string) bool {
	switch t := v.(type) {
	case map[string]any:
		if _, ok := t[key]; ok {
			return true
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if findKeyAny(t[k], key) {
				return true
			}
		}
	case []any:
		for _, e := range t {
			if findKeyAny(e, key) {
				return true
			}
		}
	}
	return false
}

// setKeyAny sets the first occurrence of key (own level first, then nested in
// sorted-key order) to val.
func setKeyAny(v any, key string, val any) bool {
	switch t := v.(type) {
	case map[string]any:
		if _, ok := t[key]; ok {
			t[key] = val
			return true
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if setKeyAny(t[k], key, val) {
				return true
			}
		}
	case []any:
		for i := range t {
			if setKeyAny(t[i], key, val) {
				return true
			}
		}
	}
	return false
}

// lookupValue returns the first value stored under key anywhere in the parsed
// JSON (sorted walk → deterministic), or nil.
func lookupValue(v any, key string) any {
	switch t := v.(type) {
	case map[string]any:
		if val, ok := t[key]; ok {
			return val
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if sub := lookupValue(t[k], key); sub != nil {
				return sub
			}
		}
	case []any:
		for _, e := range t {
			if sub := lookupValue(e, key); sub != nil {
				return sub
			}
		}
	}
	return nil
}

// collectMoneyKeys returns the distinct money-like key names in the body
// (auto mode), sorted.
func collectMoneyKeys(v any, out *[]string, seen *map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if moneyKeyRe.MatchString(k) && !(*seen)[k] {
				(*seen)[k] = true
				*out = append(*out, k)
			}
			collectMoneyKeys(val, out, seen)
		}
	case []any:
		for _, e := range t {
			collectMoneyKeys(e, out, seen)
		}
	}
}

// inflightModify fires a JSON request once as-is (baseline) and once per
// (field x mode) with the field mutated, and reports which variants the server
// accepted.
func (t *ToolSet) inflightModify() actool.CoreTool {
	return t.writeExpTool("inflight_modify",
		"Price/amount tamper screening: fire a JSON request once as-is (baseline) and once per (field x mode) with the field mutated — zero, 0.01, -1, 1, 999999999, null. Fields default to auto-detected money-like names (price/amount/total/qty/quantity/count/discount/fee/cost/balance/payment/charge); pass fields to pin exact names. Every request is captured as evidence. Screening only: an accepted tampered variant must be confirmed with a read-back (the state actually moved) before report_finding.",
		obj(map[string]any{
			"url":     str("request URL"),
			"method":  str("HTTP method (default POST)"),
			"body":    str("the JSON request body to tamper (as recorded from traffic)"),
			"headers": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "request headers as \"K: V\" lines (include the session cookie)"},
			"fields":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "exact field names to tamper (default: auto-detect money-like keys)"},
			"modes":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "tamper modes: zero, cent, negative, one, huge, null (default: all)"},
		}, "url", "body"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				URL     string   `json:"url"`
				Method  string   `json:"method"`
				Body    string   `json:"body"`
				Headers []string `json:"headers"`
				Fields  []string `json:"fields"`
				Modes   []string `json:"modes"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf("inflight_modify: bad input JSON: " + err.Error()), nil
			}
			if strings.TrimSpace(a.URL) == "" {
				return actool.Errorf("inflight_modify: url is required"), nil
			}
			session, err := t.newReplaySession(a.URL)
			if err != nil {
				return actool.Errorf("inflight_modify: " + err.Error()), nil
			}
			ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			if strings.TrimSpace(a.Body) == "" {
				return actool.Errorf("inflight_modify: body is required (the JSON request to tamper)"), nil
			}
			if a.Method == "" {
				a.Method = "POST"
			}
			var doc any
			if err := json.Unmarshal([]byte(a.Body), &doc); err != nil {
				return actool.Errorf("inflight_modify: body must be JSON (form tampering is not supported): " + err.Error()), nil
			}
			var keys []string
			if len(a.Fields) > 0 {
				for _, k := range a.Fields {
					if k = strings.TrimSpace(k); k != "" && findKeyAny(doc, k) && !strContains(keys, k) {
						keys = append(keys, k)
					}
				}
			} else {
				seen := map[string]bool{}
				collectMoneyKeys(doc, &keys, &seen)
				sort.Strings(keys)
			}
			if len(keys) == 0 {
				return actool.Errorf("inflight_modify: no tamperable field found in the body — pass fields explicitly (exact JSON key names)"), nil
			}
			if len(keys) > 4 {
				return actool.Errorf("inflight_modify: at most 4 fields per call"), nil
			}
			modes := tamperModes
			if len(a.Modes) > 0 {
				modes = nil
				for _, m := range a.Modes {
					if m = strings.TrimSpace(m); strContains(tamperModes, m) && !strContains(modes, m) {
						modes = append(modes, m)
					}
				}
				if len(modes) == 0 {
					modes = tamperModes
				}
			}
			hdrs := headerPairs(a.Headers)
			if hdrs.Get("Content-Type") == "" {
				hdrs.Set("Content-Type", "application/json")
			}
			fireOne := func(body string) fireResult {
				return session.fireHTTP(ctx, a.Method, a.URL, cloneHeader(hdrs), []byte(body))
			}

			base := fireOne(a.Body)
			rows := []map[string]any{{
				"field":   "(baseline)",
				"mode":    "-",
				"status":  base.Status,
				"ms":      base.MS,
				"bytes":   len(base.Body),
				"snippet": clipText(string(base.Body), 160),
			}}
			nAccepted := 0
			var accepted []string
			for _, k := range keys {
				var orig any
				_ = json.Unmarshal([]byte(a.Body), &orig)
				origVal := lookupValue(orig, k)
				for _, m := range modes {
					var mut any
					_ = json.Unmarshal([]byte(a.Body), &mut)
					setKeyAny(mut, k, tamperValue(origVal, m))
					b, err := json.Marshal(mut)
					if err != nil {
						continue
					}
					f := fireOne(string(b))
					row := map[string]any{
						"field":   k,
						"mode":    m,
						"value":   tamperValue(origVal, m),
						"status":  f.Status,
						"ms":      f.MS,
						"bytes":   len(f.Body),
						"snippet": clipText(string(f.Body), 160),
					}
					if f.Err != "" {
						row["error"] = f.Err
					}
					if f.Status >= 200 && f.Status < 300 {
						nAccepted++
						accepted = append(accepted, k+"="+m)
					}
					rows = append(rows, row)
				}
			}
			out := map[string]any{
				"url":        a.URL,
				"method":     a.Method,
				"fields":     keys,
				"modes":      modes,
				"n_rows":     len(rows) - 1,
				"n_accepted": nAccepted,
				"accepted":   accepted,
				"results":    rows,
				"verdict":    "ALL_REJECTED",
				"note":       "screening only: an accepted tampered variant must be confirmed with a read-back (the state actually moved) before report_finding",
			}
			if nAccepted > 0 {
				out["verdict"] = "TAMPER_ACCEPTED"
			}
			return jsonResult(out)
		})
}
