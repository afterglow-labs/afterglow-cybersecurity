package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Hunt-class close-out states. Tasks opt in to this coverage board; once
// enabled, open classes hold completion until audited or marked as a gap.
const (
	HuntClassOpen      = "open"
	HuntClassFound     = "found"
	HuntClassExhausted = "exhausted"
	HuntClassMissTag   = "miss_tag"
)

// Miss tags classify why a hunt class could not be fully audited. A miss is
// never "clean": it names the gap so the next wave can widen the surface and
// re-hunt instead of quietly declaring the class clean.
const (
	MissCoverageGap = "coverage-gap" // surface never reached the class
	MissProbeDefect = "probe-defect" // surface reached, probe failed to fire
	MissTargetDown  = "target-down"  // target unreachable in the audit window
	MissScopeOut    = "scope-out"    // class not in the authorized scope for this task
)

// ValidMissTag reports whether tag is one of the recognized miss tags.
func ValidMissTag(tag string) bool {
	switch tag {
	case MissCoverageGap, MissProbeDefect, MissTargetDown, MissScopeOut:
		return true
	}
	return false
}

// DefaultHuntClasses is the close-out class set seeded for every new task.
var DefaultHuntClasses = []string{
	"xss", "sqli", "rce", "ssrf", "idor-bola", "auth-bypass",
	"business-logic", "race", "upload", "csrf", "cors", "info-disclosure",
}

// DBHuntClass is one row of the per-task close-out scoreboard.
type DBHuntClass struct {
	ID        int64  `json:"id,string"`
	TaskID    int64  `json:"task_id,string"`
	Class     string `json:"class"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	Evidence  string `json:"evidence"`
	MissTag   string `json:"miss_tag,omitempty"`
	WidenNext string `json:"widen_next,omitempty"`
}

// SeedHuntClasses inserts the default class set for taskID, idempotently.
func (d *DB) SeedHuntClasses(taskID int64) error {
	if taskID <= 0 {
		return errors.New("task is required")
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO hunt_classes(task_id, class) VALUES($1,$2)
	 ON CONFLICT (task_id, class) DO NOTHING`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, c := range DefaultHuntClasses {
		if _, err := stmt.Exec(taskID, c); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListHuntClasses returns the task's scoreboard rows, ordered by class.
func (d *DB) ListHuntClasses(taskID int64) ([]DBHuntClass, error) {
	rows, err := d.Query(`SELECT id, task_id, class, status, reason, evidence, miss_tag, widen_next
	 FROM hunt_classes WHERE task_id=$1 ORDER BY class`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DBHuntClass, 0, len(DefaultHuntClasses))
	for rows.Next() {
		var r DBHuntClass
		if err := rows.Scan(&r.ID, &r.TaskID, &r.Class, &r.Status, &r.Reason, &r.Evidence, &r.MissTag, &r.WidenNext); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountOpenHuntClasses counts the classes still open (not yet found or audited).
func (d *DB) CountOpenHuntClasses(taskID int64) (int, error) {
	var n int
	err := d.QueryRow(`SELECT count(*) FROM hunt_classes WHERE task_id=$1 AND status='open'`, taskID).Scan(&n)
	return n, err
}

// SetHuntClassStatus writes a terminal (or re-opened) state for one class of a
// task. 'found' is reserved for the finding-recording path (MarkHuntClassFoundTx)
// and is rejected here so an audit can never claim a find without a record.
// 'exhausted' requires a written reason (surface + candidates tried, why each
// missed); 'miss_tag' requires a valid miss tag plus a reason; 'open' re-opens
// the class for re-hunting and clears the terminal fields.
func (d *DB) SetHuntClassStatus(taskID int64, class, status, reason, evidence, missTag, widenNext string) error {
	if taskID <= 0 || len(reason) > 4000 || len(evidence) > 4000 || len(widenNext) > 2000 {
		return errors.New("hunt class update exceeds size limit")
	}
	switch status {
	case HuntClassOpen, HuntClassExhausted, HuntClassMissTag:
	case HuntClassFound:
		return errors.New("status 'found' is set automatically by report_finding; report the finding instead of marking the class")
	default:
		return fmt.Errorf("invalid hunt class status %q", status)
	}
	switch status {
	case HuntClassExhausted:
		if strings.TrimSpace(reason) == "" {
			return errors.New("status exhausted requires a reason: the surface + candidates tried, and why each missed")
		}
		missTag, widenNext = "", ""
	case HuntClassMissTag:
		if !ValidMissTag(missTag) {
			return fmt.Errorf("status miss_tag requires a valid miss_tag (coverage-gap|probe-defect|target-down|scope-out), got %q", missTag)
		}
		if strings.TrimSpace(reason) == "" {
			return errors.New("status miss_tag requires a reason (what was tried, what blocked it)")
		}
	default: // HuntClassOpen — re-open, clear terminal fields
		reason, evidence, missTag, widenNext = "", "", "", ""
	}
	res, err := d.Exec(`UPDATE hunt_classes SET status=$3, reason=$4, evidence=$5, miss_tag=$6, widen_next=$7, updated_at=now()
	 WHERE task_id=$1 AND class=$2 AND status<>'found'`, taskID, class, status, reason, evidence, missTag, widenNext)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("hunt class %q is missing or already has a confirmed finding", class)
	}
	return nil
}

// MarkHuntClassFoundTx atomically marks the matching scoreboard class 'found'
// inside the finding-recording transaction, so finding + scoreboard close are
// one commit. Best-effort by contract: a finding must never fail because the
// scoreboard write failed, so the error is intentionally discarded.
func MarkHuntClassFoundTx(tx *sql.Tx, taskID int64, vulnClass string, findingID int64) {
	if taskID <= 0 || vulnClass == "" {
		return
	}
	hc := vulnClassToHuntClass(vulnClass)
	if hc == "" {
		return // finding outside the scoreboard's classes: leave the board untouched
	}
	// A missing or damaged optional scoreboard must not abort the finding's
	// transaction. PostgreSQL needs a savepoint to recover from statement errors.
	if _, err := tx.Exec(`SAVEPOINT afterglow_hunt_class`); err != nil {
		return
	}
	if _, err := tx.Exec(`UPDATE hunt_classes SET status='found', evidence=$3, updated_at=now()
	 WHERE task_id=$1 AND class=$2 AND status<>'found'`, taskID, hc, fmt.Sprintf("finding #%d", findingID)); err != nil {
		_, _ = tx.Exec(`ROLLBACK TO SAVEPOINT afterglow_hunt_class`)
	}
	_, _ = tx.Exec(`RELEASE SAVEPOINT afterglow_hunt_class`)
}

// vulnClassToHuntClass maps a finding's vuln_class label onto the scoreboard
// class (case-insensitive substring match). Returns "" for classes the
// scoreboard does not track. Order matters: more specific branches first
// (e.g. "cross-site script" before a generic "cross-site"; "authorization"
// maps to idor-bola, not auth-bypass).
func vulnClassToHuntClass(vulnClass string) string {
	v := strings.ToLower(strings.TrimSpace(vulnClass))
	if v == "" {
		return ""
	}
	switch {
	case strings.Contains(v, "xss") || strings.Contains(v, "cross-site script") || strings.Contains(v, "html injection") || strings.Contains(v, "dom-based"):
		return "xss"
	case strings.Contains(v, "sqli") || strings.Contains(v, "sql injection") || strings.Contains(v, "nosql") || strings.Contains(v, "database injection"):
		return "sqli"
	case strings.Contains(v, "rce") || strings.Contains(v, "command injection") || strings.Contains(v, "code injection") ||
		strings.Contains(v, "expression language") || strings.Contains(v, "ssti") || strings.Contains(v, "template injection") ||
		strings.Contains(v, "deserializ") || strings.Contains(v, "remote code") || strings.Contains(v, "reverse shell"):
		return "rce"
	case strings.Contains(v, "ssrf") || strings.Contains(v, "server-side request") || strings.Contains(v, "server side request"):
		return "ssrf"
	case strings.Contains(v, "idor") || strings.Contains(v, "bola") || strings.Contains(v, "bfla") || strings.Contains(v, "broken access") ||
		strings.Contains(v, "broken object") || strings.Contains(v, "authorization") || strings.Contains(v, "mass assignment") ||
		strings.Contains(v, "privilege escalation"):
		return "idor-bola"
	case strings.Contains(v, "csrf") || strings.Contains(v, "cross-site request forgery") || strings.Contains(v, "request forgery"):
		return "csrf"
	case strings.Contains(v, "auth") || strings.Contains(v, "login") || strings.Contains(v, "session") || strings.Contains(v, "jwt") ||
		strings.Contains(v, "2fa") || strings.Contains(v, "mfa") || strings.Contains(v, "account takeover") || strings.Contains(v, "credential"):
		return "auth-bypass"
	case strings.Contains(v, "race") || strings.Contains(v, "toctou") || strings.Contains(v, "concurrent") || strings.Contains(v, "replay") ||
		strings.Contains(v, "double spend") || strings.Contains(v, "double-click"):
		return "race"
	case strings.Contains(v, "business") || strings.Contains(v, "logic") || strings.Contains(v, "price") || strings.Contains(v, "payment") ||
		strings.Contains(v, "workflow") || strings.Contains(v, "state machine") || strings.Contains(v, "tamper"):
		return "business-logic"
	case strings.Contains(v, "upload") || strings.Contains(v, "file inclusion") || strings.Contains(v, "lfi") || strings.Contains(v, "path traversal") ||
		strings.Contains(v, "zip slip") || strings.Contains(v, "symlink"):
		return "upload"
	case strings.Contains(v, "cors") || strings.Contains(v, "cross-origin"):
		return "cors"
	case strings.Contains(v, "info") || strings.Contains(v, "disclosure") || strings.Contains(v, "leak") || strings.Contains(v, "exposure") ||
		strings.Contains(v, "debug") || strings.Contains(v, "misconfig") || strings.Contains(v, "open redirect") || strings.Contains(v, "xxe") ||
		strings.Contains(v, "header injection") || strings.Contains(v, "crlf") || strings.Contains(v, "clickjacking") || strings.Contains(v, "subdomain takeover"):
		return "info-disclosure"
	}
	return ""
}

// UntestedSurfaceCount counts the task's surface assets (endpoint/service/app)
// with NO fact or finding anchored to them in the task's own exploration — the
// A4 "exhaustion ≠ clean" fuel: a coverage-gap class still has untested surface
// left in this task, and the count feeds the auto-generated widen_next.
func (d *DB) UntestedSurfaceCount(taskID int64) (int, error) {
	var n int
	err := d.QueryRow(`
SELECT count(*)
FROM assets a
WHERE $1 = ANY(a.task_ids) AND a.type IN ('endpoint','service','app')
  AND NOT EXISTS (
    SELECT 1 FROM exploration_anchors ea
    JOIN exploration_nodes en ON en.id = ea.node_id
    WHERE ea.asset_id = a.id
      AND en.exploration_id = (SELECT exploration_id FROM tasks WHERE id = $1)
      AND en.kind IN ('fact','finding')
  )`, taskID).Scan(&n)
	return n, err
}

// SetHuntClassWidenNext updates only the widen_next column (A4 auto-suggestion
// path — best-effort, never fails the audit for a cosmetic write).
func (d *DB) SetHuntClassWidenNext(taskID int64, class, widenNext string) error {
	_, err := d.Exec(`UPDATE hunt_classes SET widen_next=$3, updated_at=now()
	 WHERE task_id=$1 AND class=$2 AND COALESCE(widen_next,'') = ''`, taskID, class, widenNext)
	return err
}

// ListHuntClassesForExploration returns the close-out scoreboard of the task
// that owns this exploration (tasks.exploration_id is unique).
func (s *ExplorationStore) ListHuntClassesForExploration() ([]DBHuntClass, error) {
	var taskID int64
	err := s.db.QueryRow(`SELECT id FROM tasks WHERE exploration_id=$1`, s.expID).Scan(&taskID)
	if err != nil {
		return nil, err
	}
	return s.db.ListHuntClasses(taskID)
}

// SetHuntClassStatusForExploration is the per-exploration write path used by
// agents: it resolves the owning task, then applies the validated update.
func (s *ExplorationStore) SetHuntClassStatusForExploration(class, status, reason, evidence, missTag, widenNext string) error {
	var taskID int64
	err := s.db.QueryRow(`SELECT id FROM tasks WHERE exploration_id=$1`, s.expID).Scan(&taskID)
	if err != nil {
		return err
	}
	return s.db.SetHuntClassStatus(taskID, class, status, reason, evidence, missTag, widenNext)
}
