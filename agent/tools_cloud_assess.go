package agent

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Autumn-27/artex/cloudassess"
	actool "github.com/Autumn-27/norma/tool"
)

func (t *ToolSet) enumerateOIDCChain() actool.CoreTool {
	return readTool("enumerate_oidc_chain", "Prepare a scoped OIDC trust review. Returns checks to perform; does not fetch endpoints or assume a role.",
		obj(map[string]any{"domain": str("OIDC issuer domain in this task's declared scope")}, "domain"),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			var a struct {
				Domain string `json:"domain"`
			}
			if json.Unmarshal(input, &a) != nil {
				return actool.Errorf("invalid input"), nil
			}
			domain, ok := t.targetInDeclaredScope(a.Domain)
			if !ok {
				return actool.Errorf("domain is outside this task's declared scope"), nil
			}
			return jsonResult(map[string]any{"domain": domain, "checks": []string{"Review the issuer's OIDC discovery document and JWKS URL", "Compare configured trust issuer, audience, subject, and thumbprints with the current issuer", "Inspect role policies for broad subject wildcards or stale trust", "Record evidence before claiming a role is assumable"}, "status": "plan_only"})
		})
}

func (t *ToolSet) queryIMDS() actool.CoreTool {
	return readTool("query_imds", "Plan a scoped cloud instance metadata configuration check. Does not request or reveal access tokens.",
		obj(map[string]any{"target": str("scoped instance host or IP"), "provider": str("aws, azure, or gcp"), "version": str("aws v1 or v2; optional")}, "target", "provider"),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			var a struct {
				Target   string `json:"target"`
				Provider string `json:"provider"`
				Version  string `json:"version"`
			}
			if json.Unmarshal(input, &a) != nil {
				return actool.Errorf("invalid input"), nil
			}
			host, ok := t.targetInDeclaredScope(a.Target)
			if !ok {
				return actool.Errorf("target is outside this task's declared scope"), nil
			}
			provider := strings.ToLower(strings.TrimSpace(a.Provider))
			if provider != "aws" && provider != "azure" && provider != "gcp" {
				return actool.Errorf("provider must be aws, azure, or gcp"), nil
			}
			version := strings.ToLower(strings.TrimSpace(a.Version))
			if version != "" && version != "v1" && version != "v2" {
				return actool.Errorf("version must be v1 or v2"), nil
			}
			return jsonResult(map[string]any{"target": host, "provider": provider, "version": version, "checks": []string{"Confirm access to the instance is authorized", "Verify metadata endpoint access controls and hop limits", "Record only role or identity metadata needed for the finding", "Keep any returned tokens out of tool output and reports"}, "status": "plan_only"})
		})
}

func (t *ToolSet) extractTFState() actool.CoreTool {
	return readTool("extract_tf_state", "Inspect a Terraform state JSON file already in this task workspace and list likely secret field paths, never values.",
		obj(map[string]any{"file": str("relative path inside this task workspace, maximum 2 MiB")}, "file"),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			var a struct {
				File string `json:"file"`
			}
			if json.Unmarshal(input, &a) != nil {
				return actool.Errorf("invalid input"), nil
			}
			data, err := t.readTaskFile(a.File, cloudassess.MaxJSONBytes)
			if err != nil {
				return actool.Errorf("state evidence unavailable: " + err.Error()), nil
			}
			paths, err := cloudassess.TerraformSensitivePaths(data)
			if err != nil {
				return actool.Errorf("invalid state JSON"), nil
			}
			return jsonResult(map[string]any{"file": a.File, "sensitive_paths": paths, "values_returned": false})
		})
}

func (t *ToolSet) deviceCodeInit() actool.CoreTool {
	return readTool("device_code_init", "Review an authorized device code flow configuration without starting a login or sending a prompt to another person.",
		obj(map[string]any{"tenant_domain": str("scoped tenant domain"), "client_id": str("application client ID")}, "tenant_domain", "client_id"),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			var a struct {
				TenantDomain string `json:"tenant_domain"`
				ClientID     string `json:"client_id"`
			}
			if json.Unmarshal(input, &a) != nil {
				return actool.Errorf("invalid input"), nil
			}
			domain, ok := t.targetInDeclaredScope(a.TenantDomain)
			if !ok {
				return actool.Errorf("tenant domain is outside this task's declared scope"), nil
			}
			if len(a.ClientID) < 8 || len(a.ClientID) > 128 || strings.ContainsAny(a.ClientID, " \t\r\n/?#") {
				return actool.Errorf("invalid client ID"), nil
			}
			return jsonResult(map[string]any{"tenant_domain": domain, "client_id": a.ClientID, "checks": []string{"Confirm the app registration and consent policy", "Check device code grant restrictions and conditional access", "Perform any login only with an operator-owned test account", "Do not relay codes to another person"}, "status": "plan_only"})
		})
}

func (t *ToolSet) checkMFAMethods() actool.CoreTool {
	return readTool("check_mfa_methods", "Classify authentication methods from a saved Graph response in this task workspace. Does not assert a bypass.",
		obj(map[string]any{"tenant_domain": str("scoped tenant domain"), "file": str("relative JSON file inside this task workspace")}, "tenant_domain", "file"),
		func(_ context.Context, input json.RawMessage) (actool.Result, error) {
			var a struct {
				TenantDomain string `json:"tenant_domain"`
				File         string `json:"file"`
			}
			if json.Unmarshal(input, &a) != nil {
				return actool.Errorf("invalid input"), nil
			}
			domain, ok := t.targetInDeclaredScope(a.TenantDomain)
			if !ok {
				return actool.Errorf("tenant domain is outside this task's declared scope"), nil
			}
			data, err := t.readTaskFile(a.File, cloudassess.MaxJSONBytes)
			if err != nil {
				return actool.Errorf("MFA evidence unavailable: " + err.Error()), nil
			}
			methods, err := cloudassess.AnalyzeMFAMethods(data)
			if err != nil {
				return actool.Errorf("invalid MFA methods JSON"), nil
			}
			return jsonResult(map[string]any{"tenant_domain": domain, "methods": methods, "bypass_confirmed": false})
		})
}
