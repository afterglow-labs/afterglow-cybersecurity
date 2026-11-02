package cloudassess

import (
	"strings"
	"testing"
)

func TestTerraformScanReportsPathsWithoutSecretValues(t *testing.T) {
	raw := []byte(`{"resources":[{"password":"canary-secret","public_key":"public","nested":{"api_token":"other-secret"}}]}`)
	paths, err := TerraformSensitivePaths(raw)
	if err != nil || len(paths) != 2 {
		t.Fatalf("paths=%+v err=%v", paths, err)
	}
	for _, path := range paths {
		if strings.Contains(path.Path, "canary-secret") || strings.Contains(path.Path, "other-secret") || strings.Contains(path.Path, "public_key") {
			t.Fatalf("unsafe path disclosure: %+v", path)
		}
	}
}

func TestMFAMethodsDoesNotDeclareBypass(t *testing.T) {
	raw := []byte(`{"value":[{"@odata.type":"#microsoft.graph.fido2AuthenticationMethod"},{"@odata.type":"#microsoft.graph.microsoftAuthenticatorAuthenticationMethod"}]}`)
	result, err := AnalyzeMFAMethods(raw)
	if err != nil || len(result.PhishingResistant) != 1 || len(result.Review) != 1 {
		t.Fatalf("MFA analysis=%+v err=%v", result, err)
	}
}
