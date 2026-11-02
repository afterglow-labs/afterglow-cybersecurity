package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const MaxAutoWaves = 3

type AutoContinueWave struct {
	ID            int64     `json:"id,string"`
	RootTaskID    int64     `json:"root_task_id,string"`
	ParentTaskID  int64     `json:"parent_task_id,string"`
	WaveNumber    int       `json:"wave_number"`
	Reason        string    `json:"reason"`
	Detail        string    `json:"detail"`
	ScheduledAt   time.Time `json:"scheduled_at"`
	Status        string    `json:"status"`
	SpawnedTaskID *int64    `json:"spawned_task_id,omitempty"`
}

func validWaveReason(reason string) bool {
	return reason == "timeout" || reason == "open-goals" || reason == "open-intents"
}

func (d *DB) ScheduleAutoContinueWave(parentTaskID int64, reason, detail string, due time.Time) (int64, error) {
	if parentTaskID <= 0 || !validWaveReason(reason) || len(detail) > 4000 {
		return 0, errors.New("invalid follow-up wave")
	}
	root, number := parentTaskID, 1
	var inheritedRoot int64
	var inheritedNumber int
	err := d.QueryRow(`SELECT w.root_task_id, w.wave_number
FROM tasks parent JOIN auto_continue_waves w ON w.id=parent.auto_wave_id
WHERE parent.id=$1 AND parent.deleted_at IS NULL`, parentTaskID).Scan(&inheritedRoot, &inheritedNumber)
	if err == nil {
		root, number = inheritedRoot, inheritedNumber+1
	} else if err != sql.ErrNoRows {
		return 0, err
	}
	if number > MaxAutoWaves {
		return 0, nil
	}
	var id int64
	err = d.QueryRow(`INSERT INTO auto_continue_waves(root_task_id,parent_task_id,wave_number,reason,detail,scheduled_at)
SELECT $1,$2,$3,$4,$5,$6 WHERE EXISTS(SELECT 1 FROM tasks WHERE id=$2 AND deleted_at IS NULL)
ON CONFLICT (root_task_id,wave_number) DO NOTHING RETURNING id`, root, parentTaskID, number, reason, detail, due).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

// Claim one due row atomically. A stale claim is retried after restart; the
// child task's unique auto_wave_id prevents duplicate task creation.
func (d *DB) ClaimDueAutoContinueWave(now time.Time) (*AutoContinueWave, error) {
	row := d.QueryRow(`WITH due AS (
 SELECT id FROM auto_continue_waves
 WHERE (status='pending' AND scheduled_at <= $1)
    OR (status='spawning' AND updated_at < $1 - interval '10 minutes')
 ORDER BY scheduled_at,id FOR UPDATE SKIP LOCKED LIMIT 1
)
UPDATE auto_continue_waves w SET status='spawning', updated_at=$1
FROM due WHERE w.id=due.id
RETURNING w.id,w.root_task_id,w.parent_task_id,w.wave_number,w.reason,w.detail,w.scheduled_at,w.status,w.spawned_task_id`, now)
	var wave AutoContinueWave
	err := row.Scan(&wave.ID, &wave.RootTaskID, &wave.ParentTaskID, &wave.WaveNumber, &wave.Reason, &wave.Detail, &wave.ScheduledAt, &wave.Status, &wave.SpawnedTaskID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &wave, nil
}

func (d *DB) TaskIDForAutoWave(waveID int64) (int64, error) {
	var taskID int64
	err := d.QueryRow(`SELECT id FROM tasks WHERE auto_wave_id=$1`, waveID).Scan(&taskID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return taskID, err
}

func (d *DB) AutoWaveNeedsBootstrap(taskID int64) (bool, error) {
	var needed bool
	err := d.QueryRow(`SELECT (t.auto_wave_id IS NOT NULL OR t.chain_verify_id IS NOT NULL) AND NOT EXISTS (
 SELECT 1 FROM exploration_nodes n WHERE n.exploration_id=t.exploration_id AND n.kind='goal'
) FROM tasks t WHERE t.id=$1 AND t.deleted_at IS NULL`, taskID).Scan(&needed)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return needed, err
}

func (d *DB) MarkAutoWaveSpawned(waveID, taskID int64) error {
	if waveID <= 0 || taskID <= 0 {
		return errors.New("invalid follow-up wave task")
	}
	res, err := d.Exec(`UPDATE auto_continue_waves SET status='spawned', spawned_task_id=$2, updated_at=now()
WHERE id=$1 AND status='spawning'`, waveID, taskID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("follow-up wave %d was not claimed", waveID)
	}
	return nil
}

func (d *DB) DeferAutoWave(waveID int64, delay time.Duration) error {
	_, err := d.Exec(`UPDATE auto_continue_waves SET status='pending', scheduled_at=$2, updated_at=now()
WHERE id=$1 AND status='spawning'`, waveID, time.Now().Add(delay))
	return err
}

func (d *DB) CancelAutoWave(waveID int64) error {
	_, err := d.Exec(`UPDATE auto_continue_waves SET status='cancelled', updated_at=now()
WHERE id=$1 AND status IN ('pending','spawning')`, waveID)
	return err
}

func (d *DB) CancelPendingAutoWaves() error {
	_, err := d.Exec(`UPDATE auto_continue_waves SET status='cancelled', updated_at=now()
WHERE status='pending'`)
	return err
}

func (d *DB) ListAutoWavesForTask(taskID int64) ([]AutoContinueWave, error) {
	rows, err := d.Query(`SELECT id,root_task_id,parent_task_id,wave_number,reason,detail,scheduled_at,status,spawned_task_id
FROM auto_continue_waves WHERE parent_task_id=$1 ORDER BY id DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AutoContinueWave{}
	for rows.Next() {
		var wave AutoContinueWave
		if err := rows.Scan(&wave.ID, &wave.RootTaskID, &wave.ParentTaskID, &wave.WaveNumber, &wave.Reason, &wave.Detail, &wave.ScheduledAt, &wave.Status, &wave.SpawnedTaskID); err != nil {
			return nil, err
		}
		out = append(out, wave)
	}
	return out, rows.Err()
}
