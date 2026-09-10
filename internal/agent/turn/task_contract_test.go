package turn

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
)

func TestTaskContractOwnsInputAndReturnedSlices(t *testing.T) {
	constraints := []Constraint{{Text: "Do not retarget the request."}}
	obligations := []Obligation{{
		ID: "lookup", Kind: ObligationTool, PrimaryIntent: "user_lookup", Subject: " @Alice ", Required: true,
		Effect: contract.ReadOnly,
	}}
	objective := "Find Alice exactly as requested."
	task, err := NewTaskContract("request-1", objective, constraints, obligations)
	if err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256([]byte(objective))
	if task.RequestHash != hex.EncodeToString(wantHash[:]) || task.Objective != objective {
		t.Fatalf("task=%#v", task)
	}
	constraints[0].Text = "mutated"
	obligations[0].Subject = "mallory"
	if task.Constraints()[0].Text != "Do not retarget the request." || task.Obligations()[0].Subject != "alice" {
		t.Fatalf("constructor retained caller storage: %#v", task)
	}
	returnedConstraints := task.Constraints()
	returnedObligations := task.Obligations()
	returnedConstraints[0].Text = "changed"
	returnedObligations[0].DependsOn = append(returnedObligations[0].DependsOn, "other")
	if task.Constraints()[0].Text != "Do not retarget the request." || len(task.Obligations()[0].DependsOn) != 0 {
		t.Fatalf("accessor exposed internal storage: %#v", task)
	}
}

func TestTaskContractRejectsInvalidObligationsAndDependencyCycles(t *testing.T) {
	tests := []struct {
		name        string
		obligations []Obligation
	}{
		{name: "blank id", obligations: []Obligation{{Kind: ObligationAnswer, Required: true}}},
		{name: "duplicate id", obligations: []Obligation{{ID: "same", Kind: ObligationAnswer}, {ID: "same", Kind: ObligationAnswer}}},
		{name: "unknown dependency", obligations: []Obligation{{ID: "one", Kind: ObligationAnswer, DependsOn: []string{"missing"}}}},
		{name: "dependency cycle", obligations: []Obligation{
			{ID: "one", Kind: ObligationAnswer, DependsOn: []string{"two"}},
			{ID: "two", Kind: ObligationAnswer, DependsOn: []string{"one"}},
		}},
		{name: "tool obligation without tool", obligations: []Obligation{{ID: "one", Kind: ObligationTool}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewTaskContract("request", "objective", nil, tc.obligations); err == nil {
				t.Fatalf("accepted obligations=%#v", tc.obligations)
			}
		})
	}
}

func TestTaskStateSatisfiesOnlyMatchingVerifiedOrderedEvidence(t *testing.T) {
	task, err := NewTaskContract("request", "look up Alice, notify Alice, then report", nil, []Obligation{
		{ID: "lookup", Kind: ObligationTool, PrimaryIntent: "user_lookup", Subject: "alice", Required: true, Effect: contract.ReadOnly},
		{ID: "notify", Kind: ObligationTool, PrimaryIntent: "user_notification", Subject: "alice", Required: true, Effect: contract.Action, RequiresReceipt: true, DependsOn: []string{"lookup"}},
		{ID: "report", Kind: ObligationAnswer, Required: true, DependsOn: []string{"notify"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := NewTaskState(task)
	call := func(id, name, subject string) execution.Call {
		return execution.Call{ID: id, Name: name, Arguments: json.RawMessage(`{"subject":"` + subject + `"}`)}
	}

	// A later obligation cannot complete out of order.
	if err := state.Observe("user_notification", call("early", "notify_user_v2", "alice"), contract.ActionSuccessResult("early", "notify_user_v2", map[string]any{"ok": true}, 1)); err != nil {
		t.Fatal(err)
	}
	if state.Satisfied("notify") {
		t.Fatal("dependent obligation completed before its dependency")
	}
	// Wrong subject, tool errors, and unknown action outcomes never satisfy.
	for _, observation := range []struct {
		tool   string
		call   execution.Call
		result contract.Result
	}{
		{tool: "user_lookup", call: call("wrong", "lookup_user_v2", "bob"), result: contract.SuccessResult("wrong", "lookup_user_v2", map[string]any{"found": true})},
		{tool: "user_lookup", call: call("error", "lookup_user_v2", "alice"), result: contract.ErrorResult("error", "lookup_user_v2", "NOT_FOUND", "missing")},
		{tool: "user_notification", call: call("unknown", "notify_user_v2", "alice"), result: contract.ErrorResult("unknown", "notify_user_v2", "ACTION_OUTCOME_UNKNOWN", "unknown")},
	} {
		if err := state.Observe(observation.tool, observation.call, observation.result); err != nil {
			t.Fatal(err)
		}
	}
	if state.Satisfied("lookup") || state.Satisfied("notify") || !state.HasUnknownActionOutcome() {
		t.Fatalf("state accepted invalid evidence: pending=%#v", state.Pending())
	}

	if err := state.Observe("user_lookup", call("lookup-call", "lookup_user_v2", "@ALICE"), contract.SuccessResult("lookup-call", "lookup_user_v2", map[string]any{"found": true})); err != nil {
		t.Fatal(err)
	}
	if !state.Satisfied("lookup") {
		t.Fatal("matching read did not satisfy lookup")
	}
	// A committed action without its required delivery receipt is insufficient.
	if err := state.Observe("user_notification", call("no-receipt", "notify_user_v2", "alice"), contract.ActionSuccessResult("no-receipt", "notify_user_v2", map[string]any{"ok": true}, 0)); err != nil {
		t.Fatal(err)
	}
	if state.Satisfied("notify") {
		t.Fatal("action without receipt satisfied obligation")
	}
	if err := state.Observe("user_notification", call("notify-call", "notify_user_v2", "alice"), contract.ActionSuccessResult("notify-call", "notify_user_v2", map[string]any{"ok": true}, 1)); err != nil {
		t.Fatal(err)
	}
	if !state.Satisfied("notify") || state.Satisfied("report") {
		t.Fatalf("ordered reducer state is wrong: pending=%#v", state.Pending())
	}
	if !state.ObserveAnswer("complete report") || !state.Satisfied("report") || len(state.Pending()) != 0 {
		t.Fatalf("answer did not complete remaining obligation: pending=%#v", state.Pending())
	}
	if len(state.Evidence()) != 7 {
		t.Fatalf("evidence records=%d, want append-only history of 7", len(state.Evidence()))
	}
}
