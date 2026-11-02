package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	actool "github.com/Autumn-27/norma/tool"
)

// attackChain is the C1 chain tool: compose attack chains from the task's
// confirmed findings, and record their verification outcomes. ChainSmith uses
// it to publish proposed chains; the chain-verification task uses it to close
// them as verified (end-to-end proof) or rejected (which step broke, why).
func (t *ToolSet) attackChain() actool.CoreTool {
	return t.writeExpTool("attack_chain",
		"Compose and audit attack chains from this task's CONFIRMED findings. action=list: every chain with steps/status/impact. action=add: record a proposed chain — name; finding_ids: confirmed finding ids from list_findings; steps: ordered array of {step, description, from_finding?, evidence?}; prerequisites; impact P0 | P1 | P2. action=status is allowed only from the chain's assigned verification task: verified requires specific end-to-end proof, rejected requires the broken step and reason. A proposed chain is a hypothesis, not a confirmed exploit.",
		obj(map[string]any{
			"action": str("list | add | status"),
			"name":   str("for add: chain name, e.g. 'IDOR on orders -> price tamper -> free checkout'"),
			"finding_ids": map[string]any{
				"type": "array", "items": map[string]any{"type": "integer"},
				"description": "for add: confirmed finding ids (from list_findings) this chain builds on",
			},
			"steps": map[string]any{
				"type": "array",
				"items": obj(map[string]any{
					"step":         str("short step name"),
					"description":  str("what happens, which endpoint/field, why the prior step enables it"),
					"from_finding": str("the finding id that makes this step possible (optional)"),
					"evidence":     str("request/response snippet supporting this step (optional)"),
				}, "step", "description"),
			},
			"prerequisites": str("for add: what the attacker needs before step 1 (accounts, tokens, timing window)"),
			"impact":        str("for add: P0 | P1 | P2"),
			"chain_id":      idp("for status: chain id from action=list"),
			"status":        str("for status: verified | rejected"),
			"reason":        str("for status: verified = end-to-end proof per step; rejected = which step broke and why"),
		}),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				Action     string            `json:"action"`
				Name       string            `json:"name"`
				FindingIDs []json.RawMessage `json:"finding_ids"`
				Steps      json.RawMessage   `json:"steps"`
				Prereqs    string            `json:"prerequisites"`
				Impact     string            `json:"impact"`
				ChainID    json.RawMessage   `json:"chain_id"`
				Status     string            `json:"status"`
				Reason     string            `json:"reason"`
			}
			_ = json.Unmarshal(in, &a)
			switch strings.TrimSpace(a.Action) {
			case "", "list":
				chains, err := t.ts.ListChainsForExploration()
				if err != nil {
					return actool.Errorf(err.Error()), nil
				}
				items := make([]map[string]any, 0, len(chains))
				for _, c := range chains {
					item := map[string]any{
						"id": c.ID, "name": c.Name, "impact": c.Impact, "status": c.Status,
						"source_finding_ids": c.SourceFindingIDs, "steps": c.Steps,
					}
					if c.Prerequisites != "" {
						item["prerequisites"] = c.Prerequisites
					}
					if c.Reason != "" {
						item["reason"] = c.Reason
					}
					if c.SpawnedTaskID != nil {
						item["spawned_task_id"] = *c.SpawnedTaskID
					}
					items = append(items, item)
				}
				return jsonResult(map[string]any{"chains": items})
			case "add":
				ids := pidList(a.FindingIDs)
				if len(ids) == 0 {
					return actool.Errorf("add requires finding_ids: the confirmed findings (from list_findings) this chain builds on"), nil
				}
				id, err := t.ts.AddChainForExploration(a.Name, ids, a.Steps, a.Prereqs, strings.TrimSpace(a.Impact))
				if err != nil {
					return actool.Errorf(err.Error()), nil
				}
				return actool.Text(fmt.Sprintf("attack chain #%d recorded (%s, impact %s)", id, strings.TrimSpace(a.Name), a.Impact)), nil
			case "status":
				cid := pid(a.ChainID)
				if cid <= 0 {
					return actool.Errorf("status requires a valid chain_id (from action=list)"), nil
				}
				st := strings.TrimSpace(a.Status)
				if err := t.ts.SetChainStatusForExploration(cid, st, a.Reason); err != nil {
					return actool.Errorf(err.Error()), nil
				}
				if t.notifyChainStatus != nil && (st == "verified" || st == "rejected") {
					if assigned, err := t.ts.VerificationChain(); err == nil && assigned != nil {
						t.notifyChainStatus(assigned.TaskID, cid, st)
					}
				}
				return actool.Text(fmt.Sprintf("attack chain #%d set to %s", cid, st)), nil
			default:
				return actool.Errorf("action must be list | add | status"), nil
			}
		})
}
