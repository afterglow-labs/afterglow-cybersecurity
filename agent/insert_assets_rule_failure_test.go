package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/locale"
)

func TestInsertAssetsRuleLookupFailureAbortsWrites(t *testing.T) {
	d := testDB(t)
	t.Cleanup(func() { _ = d.Close() })
	for _, table := range []string{"asset_intercept_rules", "task_intercept_rules"} {
		t.Run(table, func(t *testing.T) {
			tools, task := gh50Tools(t, d, locale.En)
			if _, err := d.Assets().CreateTaskInterceptRule(task.ID, "allow", "exact_domain", "allowed.example", "", true); err != nil {
				t.Fatal(err)
			}
			const domain = "outside-gh64.example"
			t.Cleanup(func() {
				if _, err := d.Exec(`DELETE FROM assets WHERE domain=$1`, domain); err != nil {
					t.Error(err)
				}
			})
			// Hide only the rule table, leaving asset and auto-scope writes usable.
			if _, err := d.Exec(`ALTER TABLE ` + table + ` RENAME TO gh64_hidden_rules`); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := d.Exec(`ALTER TABLE gh64_hidden_rules RENAME TO ` + table); err != nil {
					t.Error(err)
				}
			})
			raw, err := json.Marshal(map[string]any{"assets": []any{map[string]any{"type": "root_domain", "domain": domain}}})
			if err != nil {
				t.Fatal(err)
			}
			res, err := tools.insertAssets().Call(context.Background(), raw, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !res.IsError || !strings.Contains(res.Flatten(), table) {
				t.Errorf("rule lookup failure must return its error, got IsError=%v %q", res.IsError, res.Flatten())
			}
			var assets int
			if err := d.QueryRow(`SELECT count(*) FROM assets WHERE domain=$1`, domain).Scan(&assets); err != nil {
				t.Fatal(err)
			}
			if assets != 0 {
				t.Errorf("rule lookup failure persisted %d assets, want 0", assets)
			}
			scope, err := d.Assets().ListTaskScope(task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(scope) != 0 {
				t.Errorf("rule lookup failure persisted auto-scope: %+v", scope)
			}
		})
	}
}
