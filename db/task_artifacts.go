package db

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/afterglow-labs/afterglow-cybersecurity/artifactindex"
)

type TaskArtifact struct {
	ID        int64     `json:"id,string"`
	TaskID    int64     `json:"task_id,string"`
	Path      string    `json:"path"`
	Type      string    `json:"type"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	ValueRank int       `json:"value_rank"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *ExplorationStore) IndexTaskArtifacts(taskID int64, files []artifactindex.Artifact) ([]TaskArtifact, error) {
	owner, err := s.TaskID()
	if err != nil {
		return nil, err
	}
	if taskID <= 0 || taskID != owner || len(files) > artifactindex.MaxFiles {
		return nil, errors.New("artifact task or count is invalid")
	}
	for _, file := range files {
		if !filepath.IsLocal(filepath.FromSlash(file.Path)) || len(file.Path) > 4096 || file.SizeBytes < 0 || file.SizeBytes > artifactindex.MaxFileBytes || len(file.SHA256) != 64 {
			return nil, errors.New("artifact metadata is invalid")
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	out := make([]TaskArtifact, 0, len(files))
	for _, file := range files {
		var row TaskArtifact
		err := tx.QueryRow(`INSERT INTO task_artifacts(task_id,relative_path,artifact_type,size_bytes,hash_sha256,value_rank)
VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(task_id,relative_path,hash_sha256) DO UPDATE SET artifact_type=EXCLUDED.artifact_type,
size_bytes=EXCLUDED.size_bytes,value_rank=EXCLUDED.value_rank
RETURNING id,task_id,relative_path,artifact_type,size_bytes,hash_sha256,value_rank,created_at`, taskID,
			file.Path, file.Type, file.SizeBytes, strings.ToLower(file.SHA256), file.ValueRank).Scan(
			&row.ID, &row.TaskID, &row.Path, &row.Type, &row.SizeBytes, &row.SHA256, &row.ValueRank, &row.CreatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *DB) ListTaskArtifacts(taskID int64) ([]TaskArtifact, error) {
	rows, err := d.Query(`SELECT id,task_id,relative_path,artifact_type,size_bytes,hash_sha256,value_rank,created_at
FROM task_artifacts WHERE task_id=$1 ORDER BY value_rank DESC,created_at DESC LIMIT 500`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TaskArtifact{}
	for rows.Next() {
		var row TaskArtifact
		if err := rows.Scan(&row.ID, &row.TaskID, &row.Path, &row.Type, &row.SizeBytes, &row.SHA256, &row.ValueRank, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *ExplorationStore) ListTaskArtifactsForExploration() ([]TaskArtifact, error) {
	taskID, err := s.TaskID()
	if err != nil {
		return nil, err
	}
	return s.db.ListTaskArtifacts(taskID)
}
