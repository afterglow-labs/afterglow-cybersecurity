package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Attack-chain states (attack_chains.status). proposed = ChainSmith output,
// awaiting verification; verified / rejected = the chain-verification task's
// audited outcome (rejected requires a written reason).
const (
	ChainProposed = "proposed"
	ChainVerified = "verified"
	ChainRejected = "rejected"
	ChainImpactP0 = "P0" // total compromise / direct money movement
	ChainImpactP1 = "P1" // sensitive-data / money-flow logic break
	ChainImpactP2 = "P2" // chained medium impact
)

// ValidChainStatus / ValidChainImpact validate the write paths.
func ValidChainStatus(s string) bool {
	switch s {
	case ChainProposed, ChainVerified, ChainRejected:
		return true
	}
	return false
}

func ValidChainImpact(s string) bool {
	switch s {
	case ChainImpactP0, ChainImpactP1, ChainImpactP2:
		return true
	}
	return false
}

// DBAttackChain is one row of attack_chains.
type DBAttackChain struct {
	ID               int64           `json:"id,string"`
	TaskID           int64           `json:"task_id,string"`
	Name             string          `json:"name"`
	SourceFindingIDs []int64         `json:"source_finding_ids"`
	Steps            json.RawMessage `json:"steps"`
	Prerequisites    string          `json:"prerequisites,omitempty"`
	Impact           string          `json:"impact"`
	Status           string          `json:"status"`
	Reason           string          `json:"reason,omitempty"`
	SpawnedTaskID    *int64          `json:"spawned_task_id,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// chainImpactRank orders chains for review (P0 first).
func chainImpactRank(impact string) int {
	switch impact {
	case ChainImpactP0:
		return 0
	case ChainImpactP1:
		return 1
	default:
		return 2
	}
}

// AddChain records a composed attack chain for the task. steps must be a JSON
// array of {step, description, from_finding?, evidence?} objects.
func (d *DB) AddChain(taskID int64, name string, sourceFindingIDs []int64, steps json.RawMessage, prerequisites, impact string) (int64, error) {
	if taskID <= 0 {
		return 0, errors.New("chain: task_id is required")
	}
	name = trimSpace(name)
	if name == "" || len(name) > 200 || strings.ContainsAny(name, "\x00\r\n") {
		return 0, errors.New("chain: name is required")
	}
	if len(steps) == 0 {
		steps = json.RawMessage(`[]`)
	} else if len(steps) > 16000 || !json.Valid(steps) {
		return 0, errors.New("chain: steps must be a JSON array")
	}
	var parsedSteps []struct {
		Step        string `json:"step"`
		Description string `json:"description"`
	}
	if json.Unmarshal(steps, &parsedSteps) != nil || len(parsedSteps) < 2 || len(parsedSteps) > 12 {
		return 0, errors.New("chain: 2–12 ordered steps are required")
	}
	for _, step := range parsedSteps {
		if strings.TrimSpace(step.Step) == "" || strings.TrimSpace(step.Description) == "" || len(step.Step) > 160 || len(step.Description) > 1200 {
			return 0, errors.New("chain: each step needs a short name and description")
		}
	}
	if !ValidChainImpact(impact) {
		return 0, fmt.Errorf("chain: invalid impact %q (P0|P1|P2)", impact)
	}
	if len(sourceFindingIDs) == 0 || len(sourceFindingIDs) > 20 || len(prerequisites) > 2000 {
		return 0, fmt.Errorf("chain: source_finding_ids is required (a chain builds on confirmed findings)")
	}
	seen := make(map[int64]bool, len(sourceFindingIDs))
	for _, findingID := range sourceFindingIDs {
		if findingID <= 0 || seen[findingID] {
			return 0, errors.New("chain: finding IDs must be unique and positive")
		}
		seen[findingID] = true
	}
	var confirmedCount int
	if err := d.QueryRow(`SELECT count(*) FROM findings WHERE task_id=$1 AND status='confirmed' AND id=ANY($2::bigint[])`, taskID, sourceFindingIDs).Scan(&confirmedCount); err != nil {
		return 0, err
	}
	if confirmedCount != len(sourceFindingIDs) {
		return 0, errors.New("chain: every source finding must be confirmed and belong to this task")
	}
	var id int64
	err := d.QueryRow(`
INSERT INTO attack_chains(task_id, name, source_finding_ids, steps, prerequisites, impact)
VALUES ($1,$2,$3,$4,$5,$6)
RETURNING id`, taskID, name, sourceFindingIDs, steps, prerequisites, impact).Scan(&id)
	return id, err
}

// chainSelectCols is the chain column list; source_finding_ids is scanned as
// its JSON text form (array_to_json) and parsed — pgx stdlib hands arrays back
// as strings, and the codebase scans them this way (task_archives precedent).
const chainSelectCols = `id, task_id, name, array_to_json(source_finding_ids)::text, steps, prerequisites, impact, status, reason, spawned_task_id, created_at, updated_at`

// scanChainRow scans one chain row (see chainSelectCols).
func scanChainRow(sc interface{ Scan(...any) error }) (*DBAttackChain, error) {
	var c DBAttackChain
	var idsRaw string
	if err := sc.Scan(&c.ID, &c.TaskID, &c.Name, &idsRaw, &c.Steps, &c.Prerequisites, &c.Impact, &c.Status, &c.Reason, &c.SpawnedTaskID, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	if idsRaw != "" {
		_ = json.Unmarshal([]byte(idsRaw), &c.SourceFindingIDs)
	}
	if c.SourceFindingIDs == nil {
		c.SourceFindingIDs = []int64{}
	}
	return &c, nil
}

// ListChains returns the task's chains, impact-ranked (P0 first) then newest.
func (d *DB) ListChains(taskID int64) ([]DBAttackChain, error) {
	rows, err := d.Query(`SELECT `+chainSelectCols+`
	 FROM attack_chains WHERE task_id=$1
	 ORDER BY CASE impact WHEN 'P0' THEN 0 WHEN 'P1' THEN 1 ELSE 2 END, id DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DBAttackChain{}
	for rows.Next() {
		c, err := scanChainRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// SetChainStatus writes the verification outcome. 'verified' requires
// evidence in reason; 'rejected' requires a written reason why the chain does
// not hold. 'proposed' re-opens a chain for another pass.
func (d *DB) SetChainStatus(taskID, id int64, status, reason string) error {
	if !ValidChainStatus(status) {
		return fmt.Errorf("chain: invalid status %q (proposed|verified|rejected)", status)
	}
	if status == ChainRejected && trimSpace(reason) == "" {
		return errors.New("chain: rejected requires a reason (which step broke, and why)")
	}
	if status == ChainVerified && len(strings.TrimSpace(reason)) < 40 {
		return errors.New("chain: verified requires specific end-to-end evidence")
	}
	if len(reason) > 8000 {
		return errors.New("chain: reason is too long")
	}
	res, err := d.Exec(`UPDATE attack_chains SET status=$3, reason=$4, updated_at=now()
	 WHERE task_id=$1 AND id=$2`, taskID, id, status, reason)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("chain %d not found for this task", id)
	}
	return nil
}

// BestProposedChainForFinding returns the highest-impact proposed chain of the
// task that cites findingID.
func (d *DB) BestProposedChainForFinding(taskID, findingID int64) (*DBAttackChain, error) {
	row := d.QueryRow(`SELECT `+chainSelectCols+`
	 FROM attack_chains
	 WHERE task_id=$1 AND status='proposed' AND source_finding_ids @> ARRAY[$2]::bigint[]
	 ORDER BY CASE impact WHEN 'P0' THEN 0 WHEN 'P1' THEN 1 ELSE 2 END, id DESC
	 LIMIT 1`, taskID, findingID)
	c, err := scanChainRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// MarkChainSpawned records the verification task on the chain (dedupe key).
func (d *DB) MarkChainSpawned(taskID, chainID, spawnedTaskID int64) error {
	_, err := d.Exec(`UPDATE attack_chains SET spawned_task_id=$3, updated_at=now()
	 WHERE task_id=$1 AND id=$2`, taskID, chainID, spawnedTaskID)
	return err
}

// HasChainTaskForFinding reports whether a verification task has already
// been spawned for this finding.
func (d *DB) HasChainTaskForFinding(taskID, findingID int64) (bool, error) {
	var n int
	err := d.QueryRow(`SELECT count(*) FROM attack_chains
	 WHERE task_id=$1 AND source_finding_ids @> ARRAY[$2]::bigint[] AND spawned_task_id IS NOT NULL`, taskID, findingID).Scan(&n)
	return n > 0, err
}

// CountSpawnedChainTasks counts chain-verification tasks whose parent is taskID.
func (d *DB) CountSpawnedChainTasks(parentTaskID int64) (int, error) {
	var n int
	// parent_ref is TEXT (the parent task id as a string).
	err := d.QueryRow(`SELECT count(*) FROM tasks WHERE parent_ref=$1 AND name LIKE 'chain-verify:%' AND deleted_at IS NULL`, strconv.FormatInt(parentTaskID, 10)).Scan(&n)
	return n, err
}

// --- exploration-scoped paths (agent tools resolve the owning task via TaskID) ---

// ListChainsForExploration is the agent read path for the chain board.
func (s *ExplorationStore) ListChainsForExploration() ([]DBAttackChain, error) {
	if assigned, err := s.VerificationChain(); err != nil {
		return nil, err
	} else if assigned != nil {
		return []DBAttackChain{*assigned}, nil
	}
	taskID, err := s.TaskID()
	if err != nil {
		return nil, err
	}
	return s.db.ListChains(taskID)
}

// AddChainForExploration is the agent write path for a composed chain.
func (s *ExplorationStore) AddChainForExploration(name string, sourceFindingIDs []int64, steps json.RawMessage, prerequisites, impact string) (int64, error) {
	if assigned, err := s.VerificationChain(); err != nil {
		return 0, err
	} else if assigned != nil {
		return 0, errors.New("verification tasks can only audit their assigned chain")
	}
	taskID, err := s.TaskID()
	if err != nil {
		return 0, err
	}
	return s.db.AddChain(taskID, name, sourceFindingIDs, steps, prerequisites, impact)
}

// SetChainStatusForExploration is the agent write path for a chain outcome.
func (s *ExplorationStore) SetChainStatusForExploration(chainID int64, status, reason string) error {
	if assigned, err := s.VerificationChain(); err != nil {
		return err
	} else if assigned != nil {
		if assigned.ID != chainID {
			return errors.New("chain does not belong to this verification task")
		}
		if status != ChainVerified && status != ChainRejected {
			return errors.New("verification task must record verified or rejected")
		}
		return s.db.SetChainStatus(assigned.TaskID, chainID, status, reason)
	}
	return errors.New("start a chain verification task to record a chain outcome")
}

func trimSpace(s string) string {
	return strings.TrimSpace(s)
}
