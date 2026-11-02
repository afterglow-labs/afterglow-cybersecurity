package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/internal/dbtest"
	actool "github.com/Autumn-27/norma/tool"
)

func TestProveGoalWriteFailuresAreAtomic(t *testing.T) {
	d, err := db.Open(dbtest.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	for _, failure := range []struct {
		name, table, event string
		deferred           bool
	}{
		{name: "proof_insert", table: "exploration_edges", event: "BEFORE INSERT"},
		{name: "goal_update", table: "exploration_nodes", event: "BEFORE UPDATE"},
		{name: "commit", table: "exploration_edges", event: "AFTER INSERT", deferred: true},
	} {
		t.Run(failure.name, func(t *testing.T) {
			expID, err := d.CreateExploration("prove_goal atomic write fixture", "record proof before completing")
			if err != nil {
				t.Fatal(err)
			}
			defer d.Exec(`DELETE FROM explorations WHERE id=$1`, expID)
			store := d.Exploration(expID)
			goal, err := store.AddGoal(map[string]any{"text": "controlled objective"}, "test")
			if err != nil {
				t.Fatal(err)
			}
			evidence, err := store.AddNode(db.KindFact, map[string]any{"summary": "controlled evidence"}, 0, "confirmed", "test", nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := d.Exec(`UPDATE exploration_nodes SET blocked_reason='waiting for proof' WHERE id=$1`, goal); err != nil {
				t.Fatal(err)
			}
			var beforeVersion int64
			if err := d.QueryRow(`SELECT content_version FROM exploration_nodes WHERE id=$1`, goal).Scan(&beforeVersion); err != nil {
				t.Fatal(err)
			}

			// Scope every trigger to this exploration so unrelated fixtures are unaffected.
			name := fmt.Sprintf("prove_goal_failure_%d", expID)
			if _, err := d.Exec(fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.exploration_id = %d THEN
        RAISE EXCEPTION 'injected prove_goal write failure';
    END IF;
    RETURN NEW;
END $$`, name, expID)); err != nil {
				t.Fatal(err)
			}
			defer d.Exec(fmt.Sprintf(`DROP FUNCTION %s() CASCADE`, name))
			triggerKind, deferClause := "", ""
			if failure.deferred {
				triggerKind, deferClause = "CONSTRAINT ", "DEFERRABLE INITIALLY DEFERRED"
			}
			if _, err := d.Exec(fmt.Sprintf(`CREATE %sTRIGGER %s %s ON %s %s FOR EACH ROW EXECUTE FUNCTION %s()`,
				triggerKind, name, failure.event, failure.table, deferClause, name)); err != nil {
				t.Fatal(err)
			}

			tools := NewToolSet(store, "planner")
			tools.recordTerminalProofOrigin("atomic-proof", 1)
			input, err := json.Marshal(map[string]any{"goal_id": goal, "evidence_id": evidence})
			if err != nil {
				t.Fatal(err)
			}
			res, err := tools.proveGoal().Call(context.Background(), input, &actool.ToolContext{ToolUseID: "atomic-proof"})
			if err != nil || !res.IsError || !strings.Contains(res.Flatten(), "injected prove_goal write failure") {
				t.Errorf("write failure must return a tool error with its cause: result=%+v err=%v", res, err)
			}
			var state string
			var blockedReason sql.NullString
			var version, edges int64
			if err := d.QueryRow(`SELECT state, blocked_reason, content_version FROM exploration_nodes WHERE id=$1`, goal).Scan(&state, &blockedReason, &version); err != nil {
				t.Fatal(err)
			}
			if state != "open" || !blockedReason.Valid || blockedReason.String != "waiting for proof" || version != beforeVersion {
				t.Errorf("failed proof mutated goal: state=%q reason=%+v version=%d; want open, original reason, version=%d", state, blockedReason, version, beforeVersion)
			}
			if err := d.QueryRow(`SELECT count(*) FROM exploration_edges WHERE exploration_id=$1 AND src_id=$2 AND rel=$3 AND dst_id=$4`, expID, evidence, db.RelProves, goal).Scan(&edges); err != nil {
				t.Fatal(err)
			}
			if edges != 0 {
				t.Errorf("failed proof left %d proof edges, want 0", edges)
			}
			if tools.GoalMet || tools.Reason != "" || tools.terminalProof.armed.Load() || tools.terminalProof.armedIn.Load() != 0 {
				t.Errorf("failed proof signaled completion: GoalMet=%v Reason=%q armed=%v armedIn=%d", tools.GoalMet, tools.Reason, tools.terminalProof.armed.Load(), tools.terminalProof.armedIn.Load())
			}

			if _, err := d.Exec(fmt.Sprintf(`DROP TRIGGER %s ON %s`, name, failure.table)); err != nil {
				t.Fatal(err)
			}
			// Retrying after the fault clears must persist the proof, clear the block,
			// and retain Link's idempotence and SetNodeState's version increments.
			for attempt := int64(1); attempt <= 2; attempt++ {
				res, err := tools.proveGoal().Call(context.Background(), input, &actool.ToolContext{ToolUseID: "atomic-proof"})
				if err != nil || res.IsError {
					t.Fatalf("retry %d failed: result=%+v err=%v", attempt, res, err)
				}
				if err := d.QueryRow(`SELECT state, blocked_reason, content_version FROM exploration_nodes WHERE id=$1`, goal).Scan(&state, &blockedReason, &version); err != nil {
					t.Fatal(err)
				}
				if state != "met" || blockedReason.Valid || version != beforeVersion+attempt {
					t.Errorf("retry %d: state=%q reason=%+v version=%d, want met, NULL, %d", attempt, state, blockedReason, version, beforeVersion+attempt)
				}
				if err := d.QueryRow(`SELECT count(*) FROM exploration_edges WHERE exploration_id=$1 AND src_id=$2 AND rel=$3 AND dst_id=$4`, expID, evidence, db.RelProves, goal).Scan(&edges); err != nil {
					t.Fatal(err)
				}
				if edges != 1 || !tools.GoalMet || !tools.terminalProof.armed.Load() || tools.terminalProof.armedIn.Load() != 1 {
					t.Errorf("retry %d: edges=%d GoalMet=%v armed=%v armedIn=%d", attempt, edges, tools.GoalMet, tools.terminalProof.armed.Load(), tools.terminalProof.armedIn.Load())
				}
			}
		})
	}
}
