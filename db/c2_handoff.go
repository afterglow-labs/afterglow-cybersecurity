package db

import (
	"errors"
	"strings"
	"time"
)

type C2Handoff struct {
	ID           int64     `json:"id,string"`
	TaskID       int64     `json:"task_id,string"`
	Vector       string    `json:"vector"`
	TargetHost   string    `json:"target_host"`
	TargetPath   string    `json:"target_path"`
	ShellURL     string    `json:"shell_url,omitempty"`
	BinaryArch   string    `json:"binary_arch"`
	BinarySHA256 string    `json:"binary_sha256"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func validC2Vector(v string) bool {
	switch v {
	case "webshell_upload", "echo_b64", "certutil", "bitsadmin", "wget", "curl", "smb":
		return true
	}
	return false
}

func (s *ExplorationStore) RequestC2Handoff(h C2Handoff) (*C2Handoff, error) {
	owner, err := s.TaskID()
	if err != nil {
		return nil, err
	}
	if h.TaskID != owner || !validC2Vector(h.Vector) || h.TargetHost == "" || len(h.TargetHost) > 253 ||
		h.TargetPath == "" || len(h.TargetPath) > 512 || len(h.ShellURL) > 2048 || len(h.BinarySHA256) != 64 ||
		(h.BinaryArch != "x64" && h.BinaryArch != "x86") || strings.ContainsAny(h.TargetHost+h.TargetPath, "\x00\r\n") {
		return nil, errors.New("invalid C2 handoff metadata")
	}
	err = s.db.QueryRow(`INSERT INTO c2_handoffs(task_id,vector,target_host,target_path,shell_url,binary_arch,binary_sha256)
VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id,status,created_at,updated_at`, h.TaskID, h.Vector, h.TargetHost, h.TargetPath, h.ShellURL, h.BinaryArch, h.BinarySHA256).Scan(&h.ID, &h.Status, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (d *DB) GetC2Handoff(taskID, handoffID int64) (*C2Handoff, error) {
	var h C2Handoff
	err := d.QueryRow(`SELECT id,task_id,vector,target_host,target_path,shell_url,binary_arch,binary_sha256,status,created_at,updated_at
FROM c2_handoffs WHERE task_id=$1 AND id=$2`, taskID, handoffID).Scan(&h.ID, &h.TaskID, &h.Vector, &h.TargetHost, &h.TargetPath, &h.ShellURL, &h.BinaryArch, &h.BinarySHA256, &h.Status, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (s *ExplorationStore) GetC2HandoffForExploration(handoffID int64) (*C2Handoff, error) {
	taskID, err := s.TaskID()
	if err != nil {
		return nil, err
	}
	return s.db.GetC2Handoff(taskID, handoffID)
}

func (d *DB) ListC2Handoffs(taskID int64) ([]C2Handoff, error) {
	rows, err := d.Query(`SELECT id,task_id,vector,target_host,target_path,shell_url,binary_arch,binary_sha256,status,created_at,updated_at
FROM c2_handoffs WHERE task_id=$1 ORDER BY id DESC LIMIT 100`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []C2Handoff{}
	for rows.Next() {
		var h C2Handoff
		if err := rows.Scan(&h.ID, &h.TaskID, &h.Vector, &h.TargetHost, &h.TargetPath, &h.ShellURL, &h.BinaryArch, &h.BinarySHA256, &h.Status, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (d *DB) SetC2HandoffStatus(taskID, handoffID int64, status string) (bool, error) {
	if status != "acknowledged" && status != "cancelled" {
		return false, errors.New("invalid handoff status")
	}
	res, err := d.Exec(`UPDATE c2_handoffs SET status=$3,updated_at=now()
WHERE task_id=$1 AND id=$2 AND status='requested'`, taskID, handoffID, status)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
