package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Business-map entry kinds (business_map.kind). The map is the task's recorded
// business model: who acts, where money moves, which state machines gate the
// value, what is single-use, what is priced. Business-logic and race hunts
// start from these recorded paths, not from form guesses.
const (
	BMActor        = "actor"         // {role, session_id?, auth_hint?}
	BMMoneyFlow    = "money_flow"    // {from, to, endpoint, amount_field?, currency?}
	BMStateMachine = "state_machine" // {entity, states[], transitions[{from,to,request}]}
	BMSingleUse    = "single_use"    // {value_field, consumed_by?, state_read?}
	BMPricing      = "pricing"       // {field, endpoint, unit?, currency?}
	BMNote         = "note"          // free-form recorded observation
)

// ValidBusinessMapKind reports whether kind is one of the recognized entry kinds.
func ValidBusinessMapKind(kind string) bool {
	switch kind {
	case BMActor, BMMoneyFlow, BMStateMachine, BMSingleUse, BMPricing, BMNote:
		return true
	}
	return false
}

// BusinessMapEntry is one row of a task's recorded business model.
type BusinessMapEntry struct {
	ID        int64           `json:"id,string"`
	TaskID    int64           `json:"task_id,string"`
	Kind      string          `json:"kind"`
	Name      string          `json:"name"`
	Detail    json.RawMessage `json:"detail"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// AddBusinessMap upserts one entry (task_id, kind, name) with a JSONB detail
// object. An empty detail becomes {}. The upsert keeps the map small: re-
// recording the same actor/flow/field updates it in place instead of stacking
// duplicates.
func (d *DB) AddBusinessMap(taskID int64, kind, name string, detail json.RawMessage) (int64, error) {
	kind = strings.TrimSpace(kind)
	name = strings.TrimSpace(name)
	if taskID <= 0 {
		return 0, fmt.Errorf("business map: task_id is required")
	}
	if !ValidBusinessMapKind(kind) {
		return 0, fmt.Errorf("business map: invalid kind %q (actor|money_flow|state_machine|single_use|pricing|note)", kind)
	}
	if name == "" {
		return 0, fmt.Errorf("business map: name is required")
	}
	if len(name) > 160 || strings.ContainsAny(name, "\x00\r\n") || len(detail) > 4096 {
		return 0, fmt.Errorf("business map: entry exceeds size or contains invalid characters")
	}
	if len(detail) == 0 {
		detail = json.RawMessage(`{}`)
	} else if b := bytes.TrimSpace(detail); len(b) == 0 || b[0] != '{' || !json.Valid(b) {
		return 0, fmt.Errorf("business map: detail must be a JSON object")
	}
	var id int64
	err := d.QueryRow(`
INSERT INTO business_map(task_id, kind, name, detail)
VALUES ($1,$2,$3,$4)
ON CONFLICT (task_id, kind, name) DO UPDATE SET detail=EXCLUDED.detail, updated_at=now()
RETURNING id`, taskID, kind, name, detail).Scan(&id)
	return id, err
}

// ListBusinessMap returns the task's recorded business model, ordered by kind
// then name (stable for prompt injection).
func (d *DB) ListBusinessMap(taskID int64) ([]BusinessMapEntry, error) {
	rows, err := d.Query(`SELECT id, task_id, kind, name, detail, updated_at
	 FROM business_map WHERE task_id=$1 ORDER BY kind, name`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BusinessMapEntry{}
	for rows.Next() {
		var e BusinessMapEntry
		if err := rows.Scan(&e.ID, &e.TaskID, &e.Kind, &e.Name, &e.Detail, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RemoveBusinessMap deletes one entry of the task's map.
func (d *DB) RemoveBusinessMap(taskID, id int64) error {
	res, err := d.Exec(`DELETE FROM business_map WHERE task_id=$1 AND id=$2`, taskID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("business map entry %d not found for this task", id)
	}
	return nil
}

// ClearBusinessMap empties the task's map (operator reset before a re-hunt).
func (d *DB) ClearBusinessMap(taskID int64) error {
	_, err := d.Exec(`DELETE FROM business_map WHERE task_id=$1`, taskID)
	return err
}

// ListBusinessMapForExploration resolves the owning task and lists its map —
// the read path used by the worker prompt injection.
func (s *ExplorationStore) ListBusinessMapForExploration() ([]BusinessMapEntry, error) {
	var taskID int64
	err := s.db.QueryRow(`SELECT id FROM tasks WHERE exploration_id=$1`, s.expID).Scan(&taskID)
	if err != nil {
		return nil, err
	}
	return s.db.ListBusinessMap(taskID)
}

// AddBusinessMapForExploration is the per-exploration write path used by agents.
func (s *ExplorationStore) AddBusinessMapForExploration(kind, name string, detail json.RawMessage) (int64, error) {
	var taskID int64
	err := s.db.QueryRow(`SELECT id FROM tasks WHERE exploration_id=$1`, s.expID).Scan(&taskID)
	if err != nil {
		return 0, err
	}
	return s.db.AddBusinessMap(taskID, kind, name, detail)
}

// RemoveBusinessMapForExploration is the per-exploration delete path used by agents.
func (s *ExplorationStore) RemoveBusinessMapForExploration(id int64) error {
	var taskID int64
	err := s.db.QueryRow(`SELECT id FROM tasks WHERE exploration_id=$1`, s.expID).Scan(&taskID)
	if err != nil {
		return err
	}
	return s.db.RemoveBusinessMap(taskID, id)
}

// ClearBusinessMapForExploration empties the owning task's map.
func (s *ExplorationStore) ClearBusinessMapForExploration() error {
	var taskID int64
	err := s.db.QueryRow(`SELECT id FROM tasks WHERE exploration_id=$1`, s.expID).Scan(&taskID)
	if err != nil {
		return err
	}
	return s.db.ClearBusinessMap(taskID)
}
