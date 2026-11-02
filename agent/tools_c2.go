package agent

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Autumn-27/artex/c2bridge"
	"github.com/Autumn-27/artex/db"
	actool "github.com/Autumn-27/norma/tool"
)

func c2HandoffEnabled() bool {
	v := strings.TrimSpace(os.Getenv("ARTEX_ENABLE_C2_HANDOFF"))
	return v == "1" || strings.EqualFold(v, "true")
}

func (t *ToolSet) c2StageImplant() actool.CoreTool {
	return writeTool("c2_stage_implant", "Verify an operator-configured local binary and report its hash. No bytes are copied or transmitted.",
		obj(map[string]any{"arch": str("x64 or x86; defaults to x64")}),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			if !c2HandoffEnabled() || t.ts == nil {
				return actool.Errorf("C2 handoff is disabled or task context is unavailable"), nil
			}
			var args struct {
				Arch string `json:"arch"`
			}
			if json.Unmarshal(input, &args) != nil {
				return actool.Errorf("invalid input"), nil
			}
			if args.Arch == "" {
				args.Arch = "x64"
			}
			staged, err := c2bridge.Stage(args.Arch)
			if err != nil {
				return actool.Errorf("binary verification failed: " + err.Error()), nil
			}
			return jsonResult(map[string]any{"binary": staged, "transfer": "none"})
		})
}

func (t *ToolSet) c2DeliverImplant() actool.CoreTool {
	return writeTool("c2_deliver_implant", "Prepare a scoped operator handoff for delivery of an explicitly configured binary. This tool does not generate a shell command or transmit the binary; the operator must review and acknowledge the handoff.",
		obj(map[string]any{
			"vector":      str("webshell_upload, echo_b64, certutil, bitsadmin, wget, curl, or smb"),
			"target_host": str("host in this task's declared scope"),
			"target_path": str("absolute destination path on the authorized target"),
			"shell_url":   str("optional HTTPS webshell URL on the same scoped host"),
			"arch":        str("x64 or x86; defaults to x64"),
		}, "vector", "target_host", "target_path"),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			if !c2HandoffEnabled() || t.ts == nil {
				return actool.Errorf("C2 handoff is disabled or task context is unavailable"), nil
			}
			var args struct {
				Vector     string `json:"vector"`
				TargetHost string `json:"target_host"`
				TargetPath string `json:"target_path"`
				ShellURL   string `json:"shell_url"`
				Arch       string `json:"arch"`
			}
			if json.Unmarshal(input, &args) != nil {
				return actool.Errorf("invalid input"), nil
			}
			if args.Arch == "" {
				args.Arch = "x64"
			}
			host, allowed := t.targetInOperatorScope(args.TargetHost)
			if !allowed {
				return actool.Errorf("target host requires an operator-added task scope entry"), nil
			}
			if args.ShellURL != "" {
				parsed, err := url.Parse(args.ShellURL)
				if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Hostname() != host || parsed.RawQuery != "" || parsed.Fragment != "" {
					return actool.Errorf("webshell URL must be HTTPS on the scoped target host without credentials or query parameters"), nil
				}
			}
			if args.Vector == "webshell_upload" && args.ShellURL == "" {
				return actool.Errorf("webshell_upload requires a scoped HTTPS shell URL"), nil
			}
			if !validC2TargetPath(args.TargetPath) {
				return actool.Errorf("target path must be absolute and must not contain traversal or shell metacharacters"), nil
			}
			staged, err := c2bridge.Stage(args.Arch)
			if err != nil {
				return actool.Errorf("binary verification failed: " + err.Error()), nil
			}
			taskID, err := t.ts.TaskID()
			if err != nil || taskID <= 0 {
				return actool.Errorf("task context required"), nil
			}
			handoff, err := t.ts.RequestC2Handoff(db.C2Handoff{
				TaskID: taskID, Vector: args.Vector, TargetHost: host, TargetPath: args.TargetPath,
				ShellURL:   args.ShellURL,
				BinaryArch: staged.Arch, BinarySHA256: staged.SHA256,
			})
			if err != nil {
				return actool.Errorf("handoff could not be saved: " + err.Error()), nil
			}
			return jsonResult(map[string]any{"handoff": handoff, "binary_name": staged.Name, "requires_operator_action": true, "transfer": "none"})
		})
}

func validC2TargetPath(path string) bool {
	if path == "" || len(path) > 512 || strings.ContainsAny(path, "\x00\r\n;&|$`<>\"") || strings.Contains(path, "..") {
		return false
	}
	if filepath.IsAbs(path) {
		return true
	}
	return len(path) >= 3 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) && path[1] == ':' && (path[2] == '\\' || path[2] == '/')
}

func (t *ToolSet) c2AwaitOperator() actool.CoreTool {
	return writeTool("c2_await_operator", "Read the status of a saved C2 operator handoff. Only an authenticated operator can acknowledge it in the task UI.",
		obj(map[string]any{"handoff_id": intp("saved C2 handoff ID")}, "handoff_id"),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			if !c2HandoffEnabled() || t.ts == nil {
				return actool.Errorf("C2 handoff is disabled or task context is unavailable"), nil
			}
			var args struct {
				HandoffID int64 `json:"handoff_id"`
			}
			if json.Unmarshal(input, &args) != nil || args.HandoffID <= 0 {
				return actool.Errorf("invalid handoff ID"), nil
			}
			taskID, err := t.ts.TaskID()
			if err != nil || taskID <= 0 {
				return actool.Errorf("task context required"), nil
			}
			handoff, err := t.ts.GetC2HandoffForExploration(args.HandoffID)
			if err != nil {
				return actool.Errorf("handoff not found"), nil
			}
			return jsonResult(map[string]any{"handoff": handoff, "operator_acknowledged": handoff.Status == "acknowledged"})
		})
}
