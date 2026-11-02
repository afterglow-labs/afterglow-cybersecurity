package db

import (
	"strconv"
	"strings"
)

// CrossTaskFinding is one confirmed finding from ANOTHER task that touches the
// same surface (asset domain/root_domain/ip in the caller's host set). It feeds
// the worker-prompt "Prior task intelligence" slice (B7): a re-hunt on a scope
// starts from what was already confirmed elsewhere instead of re-deriving it.
type CrossTaskFinding struct {
	FindingID int64  `json:"finding_id,string"`
	TaskID    int64  `json:"task_id,string"`
	VulnClass string `json:"vulnclass,omitempty"`
	Severity  string `json:"severity,omitempty"`
	Summary   string `json:"summary,omitempty"`
	AssetHint string `json:"asset_hint,omitempty"`
}

// ListCrossTaskFindings returns open findings of other tasks anchored to
// assets matching any of hosts, ranked by severity (critical → low) then
// newest, capped. Triaged-out rows (false_positive / ignored / duplicate /
// resolved / fixed / risk_accepted) never ride the intel — the worker prompt
// says these were CONFIRMED elsewhere, so only real, open findings qualify.
// hosts should be concrete hosts (domain/ip, lowercase); an empty set returns
// no rows.
func (d *DB) ListCrossTaskFindings(excludeTaskID int64, hosts []string, limit int) ([]CrossTaskFinding, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}
	hs := make([]string, 0, len(hosts))
	seen := map[string]bool{}
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" && len(h) <= 253 && !seen[h] && len(hs) < 50 {
			seen[h] = true
			hs = append(hs, h)
		}
	}
	if len(hs) == 0 {
		return []CrossTaskFinding{}, nil
	}
	// The inner query dedupes one row per finding (a finding can anchor to
	// several matching assets — DISTINCT ON (id) keeps the first); the outer
	// one ranks by severity then recency. (DISTINCT ON must lead the ORDER BY,
	// so the two stages cannot be merged.)
	rows, err := d.Query(`
SELECT sub.id, sub.task_id, sub.vulnclass, sub.severity, sub.summary, sub.asset_hint
FROM (
  SELECT DISTINCT ON (f.id)
         f.id, f.task_id, f.vulnclass, f.severity, f.summary,
         COALESCE(NULLIF(a.domain, ''), a.ip, '') AS asset_hint
  FROM findings f
  JOIN exploration_nodes en ON en.id = f.node_id
  JOIN exploration_anchors ea ON ea.node_id = en.id
  JOIN assets a ON a.id = ea.asset_id
  WHERE f.task_id IS NOT NULL
    AND f.task_id <> $1
    AND f.status = 'confirmed'
    AND (a.domain = ANY($2) OR a.root_domain = ANY($2) OR a.ip = ANY($2))
  ORDER BY f.id
) sub
ORDER BY
  CASE sub.severity WHEN 'critical' THEN 4 WHEN 'high' THEN 3 WHEN 'medium' THEN 2 WHEN 'low' THEN 1 ELSE 0 END DESC,
  sub.id DESC
LIMIT `+strconv.Itoa(limit), excludeTaskID, hs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CrossTaskFinding{}
	for rows.Next() {
		var f CrossTaskFinding
		if err := rows.Scan(&f.FindingID, &f.TaskID, &f.VulnClass, &f.Severity, &f.Summary, &f.AssetHint); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListCrossTaskFindingsForExploration resolves the owning task and lists
// cross-task intel for a host set — the worker-prompt read path.
func (s *ExplorationStore) ListCrossTaskFindingsForExploration(hosts []string, limit int) ([]CrossTaskFinding, error) {
	taskID, err := s.TaskID()
	if err != nil {
		return nil, err
	}
	return s.db.ListCrossTaskFindings(taskID, hosts, limit)
}
