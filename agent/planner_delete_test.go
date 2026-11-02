package agent

import (
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/locale"
)

func TestPlannerDeletedIntentTriggers(t *testing.T) {
	previous := locale.ServerDefault()
	locale.SetServerDefault(locale.En)
	t.Cleanup(func() { locale.SetServerDefault(previous) })
	for _, mode := range []string{"", "soft", "hard"} {
		t.Run("output_deletion_"+mode, func(t *testing.T) {
			text := renderTriggers(&db.ExplorationStore{}, []TriggerEvent{{
				Kind: "cancelled", IntentID: 42, Summary: "Captured worker summary",
				Detail: "Incorrect conclusion", OutputDeletion: mode,
			}})
			for _, want := range []string{"#42", "Captured worker summary", "Incorrect conclusion"} {
				if !strings.Contains(text, want) {
					t.Errorf("trigger missing %q: %s", want, text)
				}
			}
			if mode == "" {
				if !strings.Contains(text, "It will no longer execute; replan accordingly.") || strings.Contains(text, "withdrawn") {
					t.Fatalf("pre-completion wording changed: %s", text)
				}
				return
			}
			if !strings.Contains(text, "output is withdrawn by the user; do not use it in further planning.") || strings.Contains(text, "will no longer execute") {
				t.Fatalf("missing output withdrawal: %s", text)
			}
			removed := strings.Contains(text, "Its exclusively supported output has been removed from the graph")
			if removed != (mode == "hard") {
				t.Fatalf("graph removal wording for %s: %s", mode, text)
			}
		})
	}
}
