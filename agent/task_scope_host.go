package agent

import (
	"net"
	"strings"
)

func validTargetHost(raw string) (string, bool) {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return ip.String(), true
	}
	if host == "" || len(host) > 253 || strings.ContainsAny(host, "/\\:@?%# \t\r\n") {
		return "", false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return "", false
			}
		}
	}
	return host, true
}

func (t *ToolSet) targetInDeclaredScope(raw string) (string, bool) {
	return t.targetInScope(raw, false)
}

func (t *ToolSet) targetInOperatorScope(raw string) (string, bool) {
	return t.targetInScope(raw, true)
}

func (t *ToolSet) targetInScope(raw string, operatorOnly bool) (string, bool) {
	host, valid := validTargetHost(raw)
	if !valid || t.as == nil || t.taskID <= 0 {
		return "", false
	}
	rows, err := t.as.ListTaskScopeWithSources(t.taskID)
	if err != nil {
		return "", false
	}
	ip := net.ParseIP(host)
	for _, scope := range rows {
		// Auto scope follows agent-inserted assets and is not an authorization
		// boundary. C2 handoffs require a scope row entered by the operator.
		if scope.Source == "auto" || (operatorOnly && scope.Source != "manual") {
			continue
		}
		switch scope.Kind {
		case "root_domain":
			domain := strings.ToLower(strings.TrimSuffix(scope.Domain, "."))
			if ip == nil && domain != "" && (host == domain || strings.HasSuffix(host, "."+domain)) {
				return host, true
			}
		case "subdomain":
			if ip == nil && host == strings.ToLower(strings.TrimSuffix(scope.Domain, ".")) {
				return host, true
			}
		case "ip", "cidr":
			if ip != nil {
				_, network, err := net.ParseCIDR(scope.Net)
				if err == nil && network.Contains(ip) {
					return host, true
				}
			}
		}
	}
	return "", false
}
