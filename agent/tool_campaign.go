package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Autumn-27/artex/db"
	actool "github.com/Autumn-27/norma/tool"
)

// planCampaign records the planner's hypothesis tree for a goal: each branch
// is a falsifiable hypothesis with an exact probe spec and the expected
// positive signal. Idempotent per branch_id — re-planning refines
// still-untested branches, never overwrites a confirmed/clean outcome. The
// Branches remain visible as untested until an audited outcome is recorded.
func (t *ToolSet) planCampaign() actool.CoreTool {
	return t.writeExpTool("plan_campaign",
		"Record the hypothesis tree (campaign) for a goal: a list of branches, each = hypothesis + exact probe spec (method/path/params/identity) + expected positive signal. Idempotent per branch_id — re-planning refines still-untested branches and never overwrites a confirmed/clean outcome; use a fresh branch_id for a genuinely new hypothesis. Review untested branches before task completion.",
		obj(map[string]any{
			"goal": str("the goal text this campaign serves"),
			"branches": map[string]any{
				"type": "array",
				"items": obj(map[string]any{
					"branch_id":       str("short stable id, e.g. b1, b2"),
					"hypothesis":      str("the falsifiable claim being tested"),
					"probe_spec":      str("exact probe: method, path, params, identity/session"),
					"expected_signal": str("what a positive looks like (response marker / state change)"),
				}, "branch_id", "hypothesis"),
			},
		}, "goal", "branches"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				Goal     string                   `json:"goal"`
				Branches []db.CampaignBranchInput `json:"branches"`
			}
			_ = json.Unmarshal(in, &a)
			if err := t.ts.PlanCampaignBranchesForExploration(strings.TrimSpace(a.Goal), a.Branches); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return actool.Text(fmt.Sprintf("campaign recorded: %d branch(es) for goal %q", len(a.Branches), strings.TrimSpace(a.Goal))), nil
		})
}

// campaignBranches lists the task's hypothesis tree with each branch's state
// (untested / confirmed / clean) and its recorded outcome.
func (t *ToolSet) campaignBranches() actool.CoreTool {
	return t.readExpTool("campaign_branches",
		"List this task's campaign branches (hypothesis tree): hypothesis, probe spec, expected signal, and the recorded outcome (status/reason/evidence/finding_id). Use it to see which branches are still untested before planning the next probe.",
		obj(map[string]any{}),
		func(context.Context, json.RawMessage) (actool.Result, error) {
			rows, err := t.ts.ListCampaignBranchesForExploration()
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			items := make([]map[string]any, 0, len(rows))
			for _, b := range rows {
				item := map[string]any{
					"branch_id":       b.BranchID,
					"goal":            b.Goal,
					"hypothesis":      b.Hypothesis,
					"probe_spec":      b.ProbeSpec,
					"expected_signal": b.ExpectedSignal,
					"status":          b.Status,
				}
				if b.Reason != "" {
					item["reason"] = b.Reason
				}
				if b.Evidence != "" {
					item["evidence"] = b.Evidence
				}
				if b.FindingID != nil {
					item["finding_id"] = *b.FindingID
				}
				items = append(items, item)
			}
			return jsonResult(map[string]any{"branches": items})
		})
}

// recordBranchOutcome writes a branch's audited result (or re-opens it).
// confirmed = the probe fired (evidence + finding_id when reported); clean =
// the probe actually ran and missed (reason + control evidence required — a
// skipped probe is not clean); untested = re-open for re-hunting.
func (t *ToolSet) recordBranchOutcome() actool.CoreTool {
	return t.writeExpTool("record_branch_outcome",
		"Record a campaign branch's audited result. status 'confirmed' = the probe fired: evidence required (the proof snippet / request-response that fired the hypothesis); pass finding_id when the hit was reported via report_finding (links the branch to the finding for chaining). status 'clean' = the probe was actually run and missed: reason AND evidence required — the control/baseline response proving the probe ran; clean without evidence is a skipped probe and is rejected. status 'untested' re-opens a branch for re-hunting.",
		obj(map[string]any{
			"branch_id":  str("the branch id from plan_campaign / campaign_branches"),
			"status":     str("confirmed | clean | untested"),
			"reason":     str("required for clean: what was probed and why the hypothesis failed"),
			"evidence":   str("required for confirmed (proof) and clean (control/baseline response)"),
			"finding_id": idp("for confirmed: the report_finding id, when the hit was reported"),
		}, "branch_id", "status"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				BranchID  string `json:"branch_id"`
				Status    string `json:"status"`
				Reason    string `json:"reason"`
				Evidence  string `json:"evidence"`
				FindingID *int64 `json:"finding_id"`
			}
			_ = json.Unmarshal(in, &a)
			if err := t.ts.RecordBranchOutcomeForExploration(a.BranchID, strings.TrimSpace(a.Status), a.Reason, a.Evidence, a.FindingID); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return actool.Text(fmt.Sprintf("branch %s recorded as %s", strings.TrimSpace(a.BranchID), strings.TrimSpace(a.Status))), nil
		})
}
