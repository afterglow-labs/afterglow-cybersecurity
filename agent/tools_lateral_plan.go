package agent

import (
	"context"
	"encoding/json"
	"strings"

	actool "github.com/Autumn-27/norma/tool"
)

func validPlanMethod(method string, allowed ...string) bool {
	for _, candidate := range allowed {
		if method == candidate {
			return true
		}
	}
	return false
}

func (t *ToolSet) lateralScan() actool.CoreTool {
	return readTool("lateral_scan", "Prepare a scoped host and service inventory. No network scan is run by this tool.",
		obj(map[string]any{"target": str("host or IP in this task's declared scope")}, "target"),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			var a struct {
				Target string `json:"target"`
			}
			if json.Unmarshal(input, &a) != nil {
				return actool.Errorf("invalid input"), nil
			}
			target, ok := t.targetInDeclaredScope(a.Target)
			if !ok {
				return actool.Errorf("target is outside this task's declared scope"), nil
			}
			return jsonResult(map[string]any{"target": target, "checks": []string{"Review known task assets and open services", "Validate reachable management ports within the approved test window", "Record observed shares, directory services, and access boundaries", "Report only measured results"}, "status": "plan_only"})
		})
}

func (t *ToolSet) harvestCredentials() actool.CoreTool {
	return readTool("harvest_credentials", "Plan credential exposure verification for a scoped host. Does not collect, display, or save secrets; record only real test accounts with save_task_credential.",
		obj(map[string]any{"target": str("scoped host"), "method": str("lsass, sam, ntds, dpapi, kerberos, files, or history")}, "target", "method"),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			var a struct {
				Target string `json:"target"`
				Method string `json:"method"`
			}
			if json.Unmarshal(input, &a) != nil {
				return actool.Errorf("invalid input"), nil
			}
			target, ok := t.targetInDeclaredScope(a.Target)
			if !ok {
				return actool.Errorf("target is outside this task's declared scope"), nil
			}
			method := strings.ToLower(strings.TrimSpace(a.Method))
			if !validPlanMethod(method, "lsass", "sam", "ntds", "dpapi", "kerberos", "files", "history") {
				return actool.Errorf("unsupported collection method"), nil
			}
			return jsonResult(map[string]any{"target": target, "method": method, "checks": []string{"Confirm the target and credential material are covered by the engagement", "Prefer metadata and exposure proof over copying credential contents", "Use save_task_credential only for an actual authorized test account", "Keep observed secrets out of tool responses and findings"}, "credentials_saved": 0, "status": "plan_only"})
		})
}

func (t *ToolSet) moveLateral() actool.CoreTool {
	return readTool("move_lateral", "Review a scoped access path using a real task credential reference. Does not execute a remote command or reveal the credential.",
		obj(map[string]any{"target": str("scoped host"), "method": str("wmi, winrm, smb, ssh, psexec, dcom, or rdp"), "credential_id": intp("existing credential ID for this task")}, "target", "method", "credential_id"),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			var a struct {
				Target       string `json:"target"`
				Method       string `json:"method"`
				CredentialID int64  `json:"credential_id"`
			}
			if json.Unmarshal(input, &a) != nil {
				return actool.Errorf("invalid input"), nil
			}
			target, ok := t.targetInDeclaredScope(a.Target)
			if !ok {
				return actool.Errorf("target is outside this task's declared scope"), nil
			}
			method := strings.ToLower(strings.TrimSpace(a.Method))
			if !validPlanMethod(method, "wmi", "winrm", "smb", "ssh", "psexec", "dcom", "rdp") {
				return actool.Errorf("unsupported access method"), nil
			}
			if t.ts == nil || t.taskID <= 0 || a.CredentialID <= 0 {
				return actool.Errorf("task credential is required"), nil
			}
			exists, err := t.ts.HasTaskCredential(t.taskID, a.CredentialID)
			if err != nil || !exists {
				return actool.Errorf("credential does not belong to this task"), nil
			}
			return jsonResult(map[string]any{"target": target, "method": method, "credential_id": a.CredentialID, "checks": []string{"Confirm this account is allowed on the target", "Verify access with the least intrusive approved method", "Record observed access and identity without placing the password in the transcript"}, "status": "plan_only"})
		})
}

func (t *ToolSet) escalatePrivilege() actool.CoreTool {
	return readTool("escalate_privilege", "Review a scoped privilege escalation hypothesis. Does not execute a payload or assert elevated access.",
		obj(map[string]any{"target": str("scoped host"), "method": str("token_steal, service_hijack, uac_bypass, potato, or kernel")}, "target", "method"),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			var a struct {
				Target string `json:"target"`
				Method string `json:"method"`
			}
			if json.Unmarshal(input, &a) != nil {
				return actool.Errorf("invalid input"), nil
			}
			target, ok := t.targetInDeclaredScope(a.Target)
			if !ok {
				return actool.Errorf("target is outside this task's declared scope"), nil
			}
			method := strings.ToLower(strings.TrimSpace(a.Method))
			if !validPlanMethod(method, "token_steal", "service_hijack", "uac_bypass", "potato", "kernel") {
				return actool.Errorf("unsupported escalation method"), nil
			}
			return jsonResult(map[string]any{"target": target, "method": method, "checks": []string{"Identify the specific prerequisite and affected configuration", "Use a reversible proof within the authorized scope", "Confirm resulting identity before reporting escalation", "Record cleanup steps for any test artifact"}, "status": "plan_only"})
		})
}

func (t *ToolSet) domainEscalate() actool.CoreTool {
	return readTool("domain_escalate", "Review a scoped domain privilege path. Does not modify domain policy, request tickets, or assert compromise.",
		obj(map[string]any{"domain": str("domain in task scope"), "method": str("dcsync, golden_ticket, kerberoast, asrep_roast, or gpo")}, "domain", "method"),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			var a struct {
				Domain string `json:"domain"`
				Method string `json:"method"`
			}
			if json.Unmarshal(input, &a) != nil {
				return actool.Errorf("invalid input"), nil
			}
			domain, ok := t.targetInDeclaredScope(a.Domain)
			if !ok {
				return actool.Errorf("domain is outside this task's declared scope"), nil
			}
			method := strings.ToLower(strings.TrimSpace(a.Method))
			if !validPlanMethod(method, "dcsync", "golden_ticket", "kerberoast", "asrep_roast", "gpo") {
				return actool.Errorf("unsupported domain method"), nil
			}
			return jsonResult(map[string]any{"domain": domain, "method": method, "checks": []string{"Map the prerequisite privileges and trust boundary", "Inspect relevant policy and account exposure", "Use approved non-destructive verification", "Record direct evidence before claiming domain impact"}, "status": "plan_only"})
		})
}

func (t *ToolSet) checkEDR() actool.CoreTool {
	return readTool("check_edr", "Prepare a scoped endpoint protection inventory. No agent detection claim is made without observed data.",
		obj(map[string]any{"target": str("scoped host")}, "target"),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			var a struct {
				Target string `json:"target"`
			}
			if json.Unmarshal(input, &a) != nil {
				return actool.Errorf("invalid input"), nil
			}
			target, ok := t.targetInDeclaredScope(a.Target)
			if !ok {
				return actool.Errorf("target is outside this task's declared scope"), nil
			}
			return jsonResult(map[string]any{"target": target, "checks": []string{"Inspect approved endpoint management inventory", "Record observed service names and sensor health", "Avoid changing or disabling protection during assessment"}, "detected": "unknown", "status": "plan_only"})
		})
}

func (t *ToolSet) cleanupArtifacts() actool.CoreTool {
	return readTool("cleanup_artifacts", "Prepare a scoped cleanup record for test artifacts. Does not delete files or tamper with logs.",
		obj(map[string]any{"target": str("scoped host"), "paths": map[string]any{"type": "array", "items": str("absolute test artifact path")}}, "target", "paths"),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			var a struct {
				Target string   `json:"target"`
				Paths  []string `json:"paths"`
			}
			if json.Unmarshal(input, &a) != nil {
				return actool.Errorf("invalid input"), nil
			}
			target, ok := t.targetInDeclaredScope(a.Target)
			if !ok {
				return actool.Errorf("target is outside this task's declared scope"), nil
			}
			if len(a.Paths) == 0 || len(a.Paths) > 30 {
				return actool.Errorf("provide 1–30 test artifact paths"), nil
			}
			for _, path := range a.Paths {
				if !validC2TargetPath(path) {
					return actool.Errorf("invalid test artifact path"), nil
				}
			}
			return jsonResult(map[string]any{"target": target, "paths": a.Paths, "status": "review_required", "deleted": false, "note": "Operator should verify ownership and remove only test-created artifacts; preserve audit logs."})
		})
}
