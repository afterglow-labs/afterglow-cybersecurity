// Package proxygraph keeps a small, concurrency-safe pool of operator-owned
// egress proxies. A health probe connects only to each configured proxy itself.
package proxygraph

import (
	"context"
	"errors"
	"net"
	"net/url"
	"sort"
	"sync"
	"time"
)

type Policy string

const (
	RoundRobin Policy = "round_robin"
	ASNDiverse Policy = "asn_diverse"
	LeastUsed  Policy = "least_used"
)

type Proxy struct {
	ID     string  `json:"id"`
	URL    string  `json:"-"`
	Type   string  `json:"type"`
	ASN    string  `json:"asn"`
	Active bool    `json:"active"`
	Score  float64 `json:"score"`
}

type Pool struct {
	mu      sync.Mutex
	nodes   map[string]Proxy
	uses    map[string]uint64
	lastASN string
	lastID  string
}

func NewPool() *Pool {
	return &Pool{nodes: map[string]Proxy{}, uses: map[string]uint64{}}
}

func (p *Pool) Add(node Proxy) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nodes[node.ID] = node
}

func (p *Pool) Remove(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.nodes, id)
	delete(p.uses, id)
}

func (p *Pool) Snapshot() []Proxy {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Proxy, 0, len(p.nodes))
	for _, n := range p.nodes {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (p *Pool) Next(ctx context.Context, policy Policy) (Proxy, error) {
	if err := ctx.Err(); err != nil {
		return Proxy{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	active := make([]Proxy, 0, len(p.nodes))
	for _, n := range p.nodes {
		if n.Active {
			active = append(active, n)
		}
	}
	if len(active) == 0 {
		return Proxy{}, errors.New("no active egress proxies")
	}
	sort.Slice(active, func(i, j int) bool { return active[i].ID < active[j].ID })
	if policy == ASNDiverse && p.lastASN != "" {
		diverse := make([]Proxy, 0, len(active))
		for _, n := range active {
			if n.ASN != "" && n.ASN != p.lastASN {
				diverse = append(diverse, n)
			}
		}
		if len(diverse) > 0 {
			active = diverse
		}
	}
	chosen := active[0]
	for _, n := range active[1:] {
		if p.uses[n.ID] < p.uses[chosen.ID] ||
			(p.uses[n.ID] == p.uses[chosen.ID] && policy == RoundRobin && chosen.ID == p.lastID) {
			chosen = n
		}
	}
	p.uses[chosen.ID]++
	p.lastASN, p.lastID = chosen.ASN, chosen.ID
	return chosen, nil
}

// Check measures reachability of the configured proxy listener. It never sends
// a request through the proxy to a third-party host.
func (p *Pool) Check(ctx context.Context) []Proxy {
	nodes := p.Snapshot()
	for i := range nodes {
		u, err := url.Parse(nodes[i].URL)
		if err != nil {
			nodes[i].Active, nodes[i].Score = false, 0
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		start := time.Now()
		conn, err := (&net.Dialer{}).DialContext(probeCtx, "tcp", u.Host)
		cancel()
		if err != nil {
			nodes[i].Active, nodes[i].Score = false, 0
		} else {
			_ = conn.Close()
			nodes[i].Active = true
			nodes[i].Score = 1 / max(time.Since(start).Seconds(), 0.001)
		}
		p.mu.Lock()
		if current, ok := p.nodes[nodes[i].ID]; ok && current.URL == nodes[i].URL {
			p.nodes[nodes[i].ID] = nodes[i]
		}
		p.mu.Unlock()
	}
	return p.Snapshot()
}
