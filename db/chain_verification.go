package db

import "database/sql"

func (d *DB) ChainForTask(taskID, chainID int64) (*DBAttackChain, error) {
	return scanChainRow(d.QueryRow(`SELECT `+chainSelectCols+` FROM attack_chains WHERE task_id=$1 AND id=$2`, taskID, chainID))
}

func (d *DB) TaskIDForChainVerification(chainID int64) (int64, error) {
	var taskID int64
	err := d.QueryRow(`SELECT id FROM tasks WHERE chain_verify_id=$1`, chainID).Scan(&taskID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return taskID, err
}

func (d *DB) CountChainVerificationTasks(parentTaskID int64) (int, error) {
	var count int
	err := d.QueryRow(`SELECT count(*) FROM tasks WHERE chain_verify_id IN (SELECT id FROM attack_chains WHERE task_id=$1)`, parentTaskID).Scan(&count)
	return count, err
}

// VerificationChain returns the parent chain assigned to this task, if any.
func (s *ExplorationStore) VerificationChain() (*DBAttackChain, error) {
	taskID, err := s.TaskID()
	if err != nil {
		return nil, err
	}
	var chainID int64
	err = s.db.QueryRow(`SELECT chain_verify_id FROM tasks WHERE id=$1 AND chain_verify_id IS NOT NULL`, taskID).Scan(&chainID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return scanChainRow(s.db.QueryRow(`SELECT `+chainSelectCols+` FROM attack_chains WHERE id=$1`, chainID))
}
