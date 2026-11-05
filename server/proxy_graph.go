package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/afterglow-labs/afterglow-cybersecurity/db"
	"github.com/afterglow-labs/afterglow-cybersecurity/proxygraph"
	"github.com/afterglow-labs/afterglow-cybersecurity/traffic"
)

const proxyGraphSetting = "proxy_graph_config"

var proxyGraphID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type proxyGraphConfig struct {
	Policy  proxygraph.Policy `json:"policy"`
	Enabled bool              `json:"enabled"`
}

type proxyGraphState struct {
	mu     sync.RWMutex
	pool   *proxygraph.Pool
	config proxyGraphConfig
}

func loadProxyGraphState(pg *db.DB) (*proxyGraphState, error) {
	state := &proxyGraphState{pool: proxygraph.NewPool(), config: proxyGraphConfig{Policy: proxygraph.RoundRobin}}
	if pg == nil {
		return state, nil
	}
	nodes, err := pg.ProxyGraphNodes()
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		state.pool.Add(n)
	}
	if raw, ok, err := pg.GetSetting(proxyGraphSetting); err != nil {
		return nil, err
	} else if ok && raw != "" {
		if err := json.Unmarshal([]byte(raw), &state.config); err != nil {
			return nil, err
		}
		if !validProxyGraphPolicy(state.config.Policy) {
			return nil, errors.New("invalid persisted proxy graph policy")
		}
	}
	return state, nil
}

func validProxyGraphPolicy(policy proxygraph.Policy) bool {
	return policy == proxygraph.RoundRobin || policy == proxygraph.ASNDiverse || policy == proxygraph.LeastUsed
}

func (s *Server) proxyGraphNextURL() (string, error) {
	state := s.proxyGraph
	if state == nil {
		return "", nil
	}
	state.mu.RLock()
	cfg := state.config
	state.mu.RUnlock()
	if !cfg.Enabled {
		return "", nil
	}
	node, err := state.pool.Next(context.Background(), cfg.Policy)
	if err != nil {
		return "", err // configured egress never silently falls back to direct
	}
	return node.URL, nil
}

type proxyGraphNodeDTO struct {
	ID     string  `json:"id"`
	URL    string  `json:"url"`
	Type   string  `json:"type"`
	ASN    string  `json:"asn"`
	Active bool    `json:"active"`
	Score  float64 `json:"score"`
}

func proxyGraphDTO(n proxygraph.Proxy) proxyGraphNodeDTO {
	return proxyGraphNodeDTO{ID: n.ID, URL: n.URL, Type: n.Type, ASN: n.ASN, Active: n.Active, Score: n.Score}
}

func (s *Server) proxyGraphListNodes(w http.ResponseWriter, r *http.Request) {
	out := []proxyGraphNodeDTO{}
	for _, n := range s.proxyGraph.pool.Snapshot() {
		out = append(out, proxyGraphDTO(n))
	}
	writeJSON(w, 200, map[string]any{"nodes": out})
}

func validateProxyGraphURL(raw string) error {
	if len(raw) > 2048 {
		return errors.New("proxy URL is too long")
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Hostname() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("proxy URL must be a credential-free address")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return errors.New("proxy URL needs a valid port")
	}
	if _, err := traffic.ValidateProxyURL(raw); err != nil {
		return errors.New("proxy URL scheme must be http, https, or socks5")
	}
	return nil
}

func (s *Server) proxyGraphAddNode(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var body struct {
		ID, URL, Type, ASN string
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid proxy node")
		return
	}
	body.ID, body.URL, body.Type, body.ASN = strings.TrimSpace(body.ID), strings.TrimSpace(body.URL), strings.TrimSpace(body.Type), strings.TrimSpace(body.ASN)
	if !proxyGraphID.MatchString(body.ID) || len(body.ASN) > 64 {
		writeErr(w, 400, "invalid proxy ID or ASN")
		return
	}
	if body.Type == "" {
		body.Type = "datacenter"
	}
	if body.Type != "datacenter" && body.Type != "residential" && body.Type != "tor" {
		writeErr(w, 400, "invalid proxy type")
		return
	}
	if err := validateProxyGraphURL(body.URL); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	node := proxygraph.Proxy{ID: body.ID, URL: body.URL, Type: body.Type, ASN: body.ASN, Active: true, Score: 1}
	if err := s.m.pg.SaveProxyGraphNode(node); err != nil {
		writeErr(w, 500, "could not save proxy node")
		return
	}
	s.proxyGraph.pool.Add(node)
	writeJSON(w, 201, proxyGraphDTO(node))
}

func (s *Server) proxyGraphRemoveNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !proxyGraphID.MatchString(id) {
		writeErr(w, 400, "invalid proxy ID")
		return
	}
	if err := s.m.pg.DeleteProxyGraphNode(id); err != nil {
		writeErr(w, 500, "could not remove proxy node")
		return
	}
	s.proxyGraph.pool.Remove(id)
	writeJSON(w, 200, map[string]any{"removed": id})
}

func (s *Server) proxyGraphHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	nodes := s.proxyGraph.pool.Check(ctx)
	out := make([]proxyGraphNodeDTO, 0, len(nodes))
	for _, n := range nodes {
		if err := s.m.pg.UpdateProxyGraphHealth(n); err != nil {
			writeErr(w, 500, "could not save proxy health")
			return
		}
		out = append(out, proxyGraphDTO(n))
	}
	writeJSON(w, 200, map[string]any{"nodes": out})
}

func (s *Server) proxyGraphGetConfig(w http.ResponseWriter, r *http.Request) {
	s.proxyGraph.mu.RLock()
	cfg := s.proxyGraph.config
	s.proxyGraph.mu.RUnlock()
	writeJSON(w, 200, cfg)
}

func (s *Server) proxyGraphSetConfig(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var cfg proxyGraphConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil || !validProxyGraphPolicy(cfg.Policy) {
		writeErr(w, 400, "policy must be round_robin, asn_diverse, or least_used")
		return
	}
	encoded, _ := json.Marshal(cfg)
	if err := s.m.pg.SetSetting(proxyGraphSetting, string(encoded)); err != nil {
		writeErr(w, 500, "could not save proxy graph config")
		return
	}
	s.proxyGraph.mu.Lock()
	s.proxyGraph.config = cfg
	s.proxyGraph.mu.Unlock()
	writeJSON(w, 200, cfg)
}
