package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/afterglow-labs/afterglow-cybersecurity/db"
	actool "github.com/Autumn-27/norma/tool"
)

// huntClassStatus is the close-out scoreboard tool: it reads the task's
// hunt-class board, or closes/re-opens a class. The engine refuses to finish
// the task while any class is open, so this is how an audit ends a class
// without a finding. 'found' is never written here — report_finding sets it
// atomically with the finding record.
func (t *ToolSet) huntClassStatus() actool.CoreTool {
	return t.writeExpTool("hunt_class_status",
		"Close-out scoreboard of this task's hunt classes. Call with NO arguments to LIST every class with its status/reason/miss_tag. With arguments, write a class's state: status 'exhausted' = fully audited, no vuln — reason MUST describe the surface + candidates tried and why each missed (a real audit, not a guess); status 'miss_tag' = could not be audited — miss_tag (coverage-gap|probe-defect|target-down|scope-out) + reason required, and coverage-gap should also set widen_next (paths/identities/methods to try next); status 'open' re-opens a class for re-hunting. Never mark a class exhausted before actually trying its full probe corpus at every input point; a real finding closes its class automatically via report_finding.",
		obj(map[string]any{
			"class":      str("scoreboard class to update (omit to list all): " + strings.Join(db.DefaultHuntClasses, "|")),
			"status":     str("new state: open | exhausted | miss_tag"),
			"reason":     str("required for exhausted/miss_tag: surface + candidates tried, why each missed / what blocked the audit"),
			"miss_tag":   str("required for status=miss_tag: coverage-gap | probe-defect | target-down | scope-out"),
			"widen_next": str("optional: suggested surface expansion (new paths/identities/methods), mainly for coverage-gap"),
		}),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				Class     string `json:"class"`
				Status    string `json:"status"`
				Reason    string `json:"reason"`
				MissTag   string `json:"miss_tag"`
				WidenNext string `json:"widen_next"`
			}
			_ = json.Unmarshal(in, &a)
			a.Class = strings.TrimSpace(a.Class)
			if a.Class == "" {
				rows, err := t.ts.ListHuntClassesForExploration()
				if err != nil {
					return actool.Errorf(err.Error()), nil
				}
				items := make([]map[string]any, 0, len(rows))
				for _, r := range rows {
					item := map[string]any{"class": r.Class, "status": r.Status, "reason": r.Reason, "evidence": r.Evidence}
					if r.MissTag != "" {
						item["miss_tag"] = r.MissTag
					}
					if r.WidenNext != "" {
						item["widen_next"] = r.WidenNext
					}
					items = append(items, item)
				}
				return jsonResult(map[string]any{"hunt_classes": items})
			}
			if err := t.ts.SetHuntClassStatusForExploration(a.Class, strings.TrimSpace(a.Status), a.Reason, "", a.MissTag, a.WidenNext); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return actool.Text(fmt.Sprintf("hunt class %s set to %s", a.Class, strings.TrimSpace(a.Status))), nil
		})
}
