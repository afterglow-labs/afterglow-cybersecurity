package cloudassess

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const MaxJSONBytes = 2 << 20

// SensitivePath reports a likely secret-bearing field without revealing its value.
type SensitivePath struct {
	Path string `json:"path"`
	Key  string `json:"key"`
}

func TerraformSensitivePaths(raw []byte) ([]SensitivePath, error) {
	if len(raw) == 0 || len(raw) > MaxJSONBytes {
		return nil, errors.New("Terraform state must be 1 byte to 2 MiB")
	}
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	out := make([]SensitivePath, 0)
	var visit func(any, string, int)
	visit = func(value any, path string, depth int) {
		if depth > 32 || len(out) >= 100 {
			return
		}
		switch node := value.(type) {
		case map[string]any:
			for key, child := range node {
				if len(out) >= 100 {
					return
				}
				clean := strings.Map(func(r rune) rune {
					if r < 32 {
						return -1
					}
					return r
				}, key)
				if len(clean) > 120 {
					clean = clean[:120]
				}
				childPath := path + "." + clean
				if value, ok := child.(string); ok && value != "" && sensitiveKey(key) {
					out = append(out, SensitivePath{Path: childPath, Key: clean})
				} else {
					visit(child, childPath, depth+1)
				}
			}
		case []any:
			for i, child := range node {
				if len(out) >= 100 {
					return
				}
				visit(child, fmt.Sprintf("%s[%d]", path, i), depth+1)
			}
		}
	}
	visit(root, "$", 0)
	return out, nil
}

func sensitiveKey(key string) bool {
	key = strings.ToLower(key)
	if strings.Contains(key, "public") {
		return false
	}
	for _, fragment := range []string{"password", "passwd", "secret", "token", "private_key", "access_key", "api_key", "credential"} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

type MFAMethods struct {
	Methods           []string `json:"methods"`
	PhishingResistant []string `json:"phishing_resistant"`
	Review            []string `json:"review"`
}

func AnalyzeMFAMethods(raw []byte) (MFAMethods, error) {
	var out MFAMethods
	if len(raw) == 0 || len(raw) > MaxJSONBytes {
		return out, errors.New("MFA response must be 1 byte to 2 MiB")
	}
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return out, err
	}
	if object, ok := root.(map[string]any); ok {
		root = object["value"]
	}
	rows, ok := root.([]any)
	if !ok {
		return out, errors.New("MFA response must be an array or a Graph value array")
	}
	if len(rows) > 1000 {
		return out, errors.New("too many MFA methods")
	}
	out.Methods, out.PhishingResistant, out.Review = []string{}, []string{}, []string{}
	for _, row := range rows {
		object, ok := row.(map[string]any)
		if !ok {
			continue
		}
		typeName, _ := object["@odata.type"].(string)
		if len(typeName) == 0 || len(typeName) > 160 {
			continue
		}
		out.Methods = append(out.Methods, typeName)
		lower := strings.ToLower(typeName)
		if strings.Contains(lower, "fido2") || strings.Contains(lower, "windowshelloforbusiness") || strings.Contains(lower, "platformcredential") {
			out.PhishingResistant = append(out.PhishingResistant, typeName)
		} else {
			out.Review = append(out.Review, typeName)
		}
	}
	return out, nil
}
