package db

import (
	"database/sql"
	"time"
)

// Workflow is a scheduled (or manual) agent run with an optional canvas graph.
type Workflow struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	AgentKey     string     `json:"agent_key"`
	Enabled      bool       `json:"enabled"`
	ScheduleKind string     `json:"schedule_kind"` // manual | interval | daily
	IntervalSec  int        `json:"interval_sec"`
	DailyHour    int        `json:"daily_hour"`
	DailyMinute  int        `json:"daily_minute"`
	Timezone     string     `json:"timezone"`
	Prompt       string     `json:"prompt"`
	GraphJSON    string     `json:"graph_json"`
	LastFire     *time.Time `json:"last_fire,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

const workflowCols = `id, name, description, agent_key, enabled, schedule_kind, interval_sec, daily_hour, daily_minute, timezone, prompt, graph_json, last_fire, created_at, updated_at`

func scanWorkflow(sc interface{ Scan(...any) error }) (*Workflow, error) {
	var w Workflow
	var lf sql.NullTime
	if err := sc.Scan(
		&w.ID, &w.Name, &w.Description, &w.AgentKey, &w.Enabled, &w.ScheduleKind,
		&w.IntervalSec, &w.DailyHour, &w.DailyMinute, &w.Timezone, &w.Prompt, &w.GraphJSON,
		&lf, &w.CreatedAt, &w.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if lf.Valid {
		w.LastFire = &lf.Time
	}
	if w.GraphJSON == "" {
		w.GraphJSON = "{}"
	}
	return &w, nil
}

func (d *DB) ListWorkflows() ([]*Workflow, error) {
	rows, err := d.Query(`SELECT ` + workflowCols + ` FROM workflows ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Workflow
	for rows.Next() {
		w, err := scanWorkflow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (d *DB) ListEnabledWorkflows() ([]*Workflow, error) {
	rows, err := d.Query(`SELECT ` + workflowCols + ` FROM workflows WHERE enabled ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Workflow
	for rows.Next() {
		w, err := scanWorkflow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (d *DB) GetWorkflow(id int64) (*Workflow, error) {
	row := d.QueryRow(`SELECT `+workflowCols+` FROM workflows WHERE id=$1`, id)
	return scanWorkflow(row)
}

func (d *DB) CreateWorkflow(w *Workflow) (*Workflow, error) {
	if w.ScheduleKind == "" {
		w.ScheduleKind = "manual"
	}
	if w.AgentKey == "" {
		w.AgentKey = "auto"
	}
	if w.GraphJSON == "" {
		w.GraphJSON = "{}"
	}
	if w.Timezone == "" {
		w.Timezone = "Local"
	}
	row := d.QueryRow(`
INSERT INTO workflows(name, description, agent_key, enabled, schedule_kind, interval_sec, daily_hour, daily_minute, timezone, prompt, graph_json)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING `+workflowCols,
		w.Name, w.Description, w.AgentKey, w.Enabled, w.ScheduleKind,
		w.IntervalSec, w.DailyHour, w.DailyMinute, w.Timezone, w.Prompt, w.GraphJSON)
	return scanWorkflow(row)
}

func (d *DB) UpdateWorkflow(w *Workflow) error {
	_, err := d.Exec(`
UPDATE workflows SET
  name=$2, description=$3, agent_key=$4, enabled=$5, schedule_kind=$6,
  interval_sec=$7, daily_hour=$8, daily_minute=$9, timezone=$10, prompt=$11, graph_json=$12,
  updated_at=now()
WHERE id=$1`,
		w.ID, w.Name, w.Description, w.AgentKey, w.Enabled, w.ScheduleKind,
		w.IntervalSec, w.DailyHour, w.DailyMinute, w.Timezone, w.Prompt, w.GraphJSON)
	return err
}

func (d *DB) DeleteWorkflow(id int64) error {
	_, err := d.Exec(`DELETE FROM workflows WHERE id=$1`, id)
	return err
}

func (d *DB) TouchWorkflowFire(id int64) error {
	_, err := d.Exec(`UPDATE workflows SET last_fire=now(), updated_at=now() WHERE id=$1`, id)
	return err
}

// ClaimWorkflowFire atomically admits one scheduled run across server instances.
// threshold is the latest previous fire allowed for the current schedule slot.
func (d *DB) ClaimWorkflowFire(id int64, now, threshold time.Time) (bool, error) {
	var claimed int64
	err := d.QueryRow(`UPDATE workflows SET last_fire=$2, updated_at=now()
WHERE id=$1 AND enabled AND schedule_kind<>'manual' AND (last_fire IS NULL OR last_fire<$3)
RETURNING id`, id, now, threshold).Scan(&claimed)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}
