package agent

import (
	"context"
	"encoding/json"

	"github.com/Autumn-27/artex/artifactindex"
	actool "github.com/Autumn-27/norma/tool"
)

// KSIR's exfil_data returned a staged=true claim without touching any files.
// This replacement indexes real files in the task workspace and records hashes.
// It performs no outbound transfer and never returns file contents to the model.
func (t *ToolSet) indexTaskArtifacts() actool.CoreTool {
	return writeTool("exfil_data",
		"Index actual files already collected into this task's workspace. Returns a local evidence manifest with size, type, and SHA-256; no bytes are transferred out of the workspace. Files outside this task and symlinks are rejected.",
		obj(map[string]any{
			"target_path": str("file or directory inside this task's workspace; empty scans the task workspace"),
			"pattern":     str("optional filename glob, e.g. *.env"),
			"max_size_mb": intp("maximum total MiB to index, 1–50; 0 uses 50"),
		}),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			var args struct {
				TargetPath string `json:"target_path"`
				Pattern    string `json:"pattern"`
				MaxSizeMB  int    `json:"max_size_mb"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return actool.Errorf("invalid input"), nil
			}
			if t.ts == nil || t.workDir == "" || args.MaxSizeMB < 0 || args.MaxSizeMB > 50 {
				return actool.Errorf("task workspace unavailable or size limit invalid"), nil
			}
			taskID := t.taskID
			if taskID <= 0 {
				var err error
				taskID, err = t.ts.TaskID()
				if err != nil || taskID <= 0 {
					return actool.Errorf("task context required"), nil
				}
			}
			budget := int64(args.MaxSizeMB) << 20
			rows, err := artifactindex.CollectTask(t.workDir, taskID, args.TargetPath, args.Pattern, budget)
			if err != nil {
				return actool.Errorf("evidence indexing failed: " + err.Error()), nil
			}
			stored, err := t.ts.IndexTaskArtifacts(taskID, rows)
			if err != nil {
				return actool.Errorf("evidence manifest could not be saved"), nil
			}
			return jsonResult(map[string]any{"indexed": len(stored), "artifacts": stored, "transfer": "none"})
		})
}

func (t *ToolSet) listTaskArtifacts() actool.CoreTool {
	return writeTool("list_exfil_artifacts", "List the task's local evidence manifest. Returns file metadata and hashes, never file contents.",
		obj(map[string]any{}), func(_ context.Context, _ json.RawMessage) (actool.Result, error) {
			if t.ts == nil {
				return actool.Errorf("task context required"), nil
			}
			taskID, err := t.ts.TaskID()
			if err != nil || taskID <= 0 {
				return actool.Errorf("task context required"), nil
			}
			rows, err := t.ts.ListTaskArtifactsForExploration()
			if err != nil {
				return actool.Errorf("artifact manifest unavailable"), nil
			}
			return jsonResult(map[string]any{"artifacts": rows})
		})
}
