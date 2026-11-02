package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	actool "github.com/Autumn-27/norma/tool"
)

// businessMapRW is the A3 business-map tool: read / upsert / remove entries of
// the task's recorded business model. The map is injected into every worker
// system prompt, so business-logic and race hunts start from the recorded
// money paths, state machines and single-use values instead of form guesses.
func (t *ToolSet) businessMapRW() actool.CoreTool {
	return t.writeExpTool("business_map_rw",
		"Read/write the task's recorded business model. action=list: show every recorded entry. action=add: upsert one entry — kind is actor {role, session_id?, auth_hint?} | money_flow {from, to, endpoint, amount_field?, currency?} | state_machine {entity, states[], transitions[{from, to, request}]} | single_use {value_field, consumed_by?, state_read?} | pricing {field, endpoint, unit?, currency?} | note {text}; name names the entry (e.g. 'customer', 'order lifecycle', 'promo code'); re-adding the same kind+name updates it. action=remove: delete one entry by id. action=clear: empty the map. Record what you OBSERVE in the running app (endpoints that move money, state transitions with their exact requests, single-use codes, priced fields) and what the goal/description states. Keep it small and exact — it is injected into every worker prompt.",
		obj(map[string]any{
			"action": str("list | add | remove | clear"),
			"kind":   str("for add: actor | money_flow | state_machine | single_use | pricing | note"),
			"name":   str("for add: entry name, e.g. 'customer', 'order lifecycle', 'promo code'"),
			"detail": map[string]any{"type": "object", "description": "for add: kind-specific fields (see tool description)"},
			"id":     idp("for remove: entry id from action=list"),
		}),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				Action string          `json:"action"`
				Kind   string          `json:"kind"`
				Name   string          `json:"name"`
				Detail json.RawMessage `json:"detail"`
				ID     json.RawMessage `json:"id"`
			}
			_ = json.Unmarshal(in, &a)
			switch strings.TrimSpace(a.Action) {
			case "", "list":
				entries, err := t.ts.ListBusinessMapForExploration()
				if err != nil {
					return actool.Errorf(err.Error()), nil
				}
				items := make([]map[string]any, 0, len(entries))
				for _, e := range entries {
					items = append(items, map[string]any{"id": e.ID, "kind": e.Kind, "name": e.Name, "detail": e.Detail})
				}
				return jsonResult(map[string]any{"business_map": items})
			case "add":
				id, err := t.ts.AddBusinessMapForExploration(a.Kind, a.Name, a.Detail)
				if err != nil {
					return actool.Errorf(err.Error()), nil
				}
				return actool.Text(fmt.Sprintf("business map entry #%d recorded (%s %s)", id, a.Kind, a.Name)), nil
			case "remove":
				id := pid(a.ID)
				if id <= 0 {
					return actool.Errorf("remove requires a valid entry id (from action=list)"), nil
				}
				if err := t.ts.RemoveBusinessMapForExploration(id); err != nil {
					return actool.Errorf(err.Error()), nil
				}
				return actool.Text(fmt.Sprintf("business map entry #%d removed", id)), nil
			case "clear":
				if err := t.ts.ClearBusinessMapForExploration(); err != nil {
					return actool.Errorf(err.Error()), nil
				}
				return actool.Text("business map cleared"), nil
			default:
				return actool.Errorf("action must be list | add | remove | clear"), nil
			}
		})
}
