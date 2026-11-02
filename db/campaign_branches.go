package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Campaign branch states (campaign_branches.status). A branch ends either
// 'confirmed' (the probe fired — evidence + finding link) or 'clean' (the
// probe was actually run and missed — written reason + control/baseline
// evidence). A clean branch is an audited negative, never a skipped probe.
const (
	BranchUntested  = "untested"
	BranchConfirmed = "confirmed"
	BranchClean     = "clean"
)

// CampaignBranchInput is one hypothesis branch as planned by the planner.
type CampaignBranchInput struct {
	BranchID       string `json:"branch_id"`
	Hypothesis     string `json:"hypothesis"`
	ProbeSpec      string `json:"probe_spec,omitempty"`
	ExpectedSignal string `json:"expected_signal,omitempty"`
}

// DBCampaignBranch is one row of the planner's per-task hypothesis tree.
type DBCampaignBranch struct {
	ID             int64  `json:"id,string"`
	TaskID         int64  `json:"task_id,string"`
	BranchID       string `json:"branch_id"`
	Goal           string `json:"goal"`
	Hypothesis     string `json:"hypothesis"`
	ProbeSpec      string `json:"probe_spec"`
	ExpectedSignal string `json:"expected_signal"`
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	Evidence       string `json:"evidence,omitempty"`
	FindingID      *int64 `json:"finding_id,omitempty"`
}

// PlanCampaignBranches upserts a goal's hypothesis branches. Idempotent per
// (task, branch_id): a re-plan refreshes the spec of still-untested branches,
// but never overwrites a confirmed/clean outcome — an audited result only
// changes via an explicit re-open (RecordBranchOutcome with status='untested').
func (d *DB) PlanCampaignBranches(taskID int64, goal string, branches []CampaignBranchInput) error {
	if taskID <= 0 || len(branches) == 0 || len(branches) > 20 || len(goal) > 1000 {
		return errors.New("plan_campaign: task, goal, and 1–20 branches are required")
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO campaign_branches(task_id, branch_id, goal, hypothesis, probe_spec, expected_signal)
	 VALUES($1,$2,$3,$4,$5,$6)
	 ON CONFLICT (task_id, branch_id) DO UPDATE SET
	   goal=EXCLUDED.goal,
	   hypothesis=EXCLUDED.hypothesis,
	   probe_spec=EXCLUDED.probe_spec,
	   expected_signal=EXCLUDED.expected_signal,
	   updated_at=now()
	 WHERE campaign_branches.status='untested'`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, b := range branches {
		bid := strings.TrimSpace(b.BranchID)
		hyp := strings.TrimSpace(b.Hypothesis)
		if bid == "" || hyp == "" || len(bid) > 80 || len(hyp) > 1000 || len(b.ProbeSpec) > 2000 || len(b.ExpectedSignal) > 1000 || strings.ContainsAny(bid, "\x00\r\n") {
			return errors.New("plan_campaign: every branch needs a branch_id and a hypothesis")
		}
		if _, err := stmt.Exec(taskID, bid, goal, hyp, b.ProbeSpec, b.ExpectedSignal); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListCampaignBranches returns the task's hypothesis tree in creation order.
func (d *DB) ListCampaignBranches(taskID int64) ([]DBCampaignBranch, error) {
	rows, err := d.Query(`SELECT id, task_id, branch_id, goal, hypothesis, probe_spec, expected_signal,
	 status, reason, evidence, finding_id
	 FROM campaign_branches WHERE task_id=$1 ORDER BY id LIMIT 100`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DBCampaignBranch, 0, 8)
	for rows.Next() {
		var b DBCampaignBranch
		if err := rows.Scan(&b.ID, &b.TaskID, &b.BranchID, &b.Goal, &b.Hypothesis, &b.ProbeSpec,
			&b.ExpectedSignal, &b.Status, &b.Reason, &b.Evidence, &b.FindingID); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// CountUntestedBranches counts branches whose probe has not been run yet.
func (d *DB) CountUntestedBranches(taskID int64) (int, error) {
	var n int
	err := d.QueryRow(`SELECT count(*) FROM campaign_branches WHERE task_id=$1 AND status='untested'`, taskID).Scan(&n)
	return n, err
}

// RecordBranchOutcome writes a branch's audited result (or re-opens it).
//   - confirmed: evidence required; finding_id (when given) must be a finding
//     of this task — it links the branch to the finding record for chaining.
//   - clean: reason AND evidence required — the control/baseline snippet that
//     proves the probe actually ran and missed. Clean without evidence is a
//     skipped probe, which the gate must not accept as terminal.
//   - untested: re-opens the branch for re-hunting; clears the outcome fields.
func (d *DB) RecordBranchOutcome(taskID int64, branchID, status, reason, evidence string, findingID *int64) error {
	branchID = strings.TrimSpace(branchID)
	if taskID <= 0 || branchID == "" || len(branchID) > 80 || len(reason) > 2000 || len(evidence) > 4000 {
		return errors.New("record_branch_outcome: branch_id required")
	}
	switch status {
	case BranchConfirmed:
		if strings.TrimSpace(evidence) == "" {
			return errors.New("status confirmed requires evidence (the proof snippet / request-response that fired the hypothesis)")
		}
		if findingID != nil {
			var ftask *int64
			var findingStatus string
			err := d.QueryRow(`SELECT task_id,status FROM findings WHERE id=$1`, *findingID).Scan(&ftask, &findingStatus)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("finding #%d not found", *findingID)
			}
			if err != nil {
				return err
			}
			if ftask == nil || *ftask != taskID || findingStatus != FindingConfirmed {
				return fmt.Errorf("finding #%d must be confirmed and belong to this task", *findingID)
			}
		}
		reason = "" // confirmed keeps evidence + finding; reason cleared
	case BranchClean:
		if strings.TrimSpace(reason) == "" {
			return errors.New("status clean requires a reason (what was probed, why the hypothesis failed)")
		}
		if strings.TrimSpace(evidence) == "" {
			return errors.New("status clean requires evidence (the control/baseline response showing the probe ran and missed)")
		}
		findingID = nil
	case BranchUntested:
		reason, evidence, findingID = "", "", nil
	default:
		return fmt.Errorf("invalid branch status %q (untested|confirmed|clean)", status)
	}
	res, err := d.Exec(`UPDATE campaign_branches SET status=$3, reason=$4, evidence=$5, finding_id=$6, updated_at=now()
	 WHERE task_id=$1 AND branch_id=$2`, taskID, branchID, status, reason, evidence, findingID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("branch %q not found for this task — plan it first", branchID)
	}
	return nil
}

// ListCampaignBranchesForExploration returns the hypothesis tree of the task
// that owns this exploration (tasks.exploration_id is unique).
func (s *ExplorationStore) ListCampaignBranchesForExploration() ([]DBCampaignBranch, error) {
	var taskID int64
	err := s.db.QueryRow(`SELECT id FROM tasks WHERE exploration_id=$1`, s.expID).Scan(&taskID)
	if err != nil {
		return nil, err
	}
	return s.db.ListCampaignBranches(taskID)
}

// PlanCampaignBranchesForExploration is the planner-side write path.
func (s *ExplorationStore) PlanCampaignBranchesForExploration(goal string, branches []CampaignBranchInput) error {
	var taskID int64
	err := s.db.QueryRow(`SELECT id FROM tasks WHERE exploration_id=$1`, s.expID).Scan(&taskID)
	if err != nil {
		return err
	}
	return s.db.PlanCampaignBranches(taskID, goal, branches)
}

// RecordBranchOutcomeForExploration is the worker/planner-side outcome path.
func (s *ExplorationStore) RecordBranchOutcomeForExploration(branchID, status, reason, evidence string, findingID *int64) error {
	var taskID int64
	err := s.db.QueryRow(`SELECT id FROM tasks WHERE exploration_id=$1`, s.expID).Scan(&taskID)
	if err != nil {
		return err
	}
	return s.db.RecordBranchOutcome(taskID, branchID, status, reason, evidence, findingID)
}
