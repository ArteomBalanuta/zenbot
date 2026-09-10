package execution

import (
	"encoding/json"
	"testing"

	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
)

func TestBatchPlannerFansOutOnlyIndependentReadOnlyCalls(t *testing.T) {
	readOne := &fake{name: "read_one", d: desc(t, "read_one", contract.ReadOnly, []string{"room:one"}, nil, true, 0, nil)}
	readTwo := &fake{name: "read_two", d: desc(t, "read_two", contract.ReadOnly, []string{"room:two"}, nil, true, 0, nil)}
	action := &fake{name: "action", d: desc(t, "action", contract.Action, nil, []string{"room:one"}, false, 0, nil)}
	registry := tool.NewRegistry([]tool.Tool{readOne, readTwo, action}, []string{"read_one", "read_two", "action"})
	planner := BatchPlanner{Registry: registry}

	stages, err := planner.Plan(ctx(t), []Call{
		{ID: "1", Name: "read_one", Arguments: json.RawMessage(`{}`)},
		{ID: "2", Name: "read_two", Arguments: json.RawMessage(`{}`)},
		{ID: "3", Name: "action", Arguments: json.RawMessage(`{}`)},
		{ID: "4", Name: "read_one", Arguments: json.RawMessage(`{}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 3 {
		t.Fatalf("stage count=%d, want 3: %#v", len(stages), stages)
	}
	if !stages[0].Parallel || len(stages[0].Calls) != 2 {
		t.Fatalf("first stage=%#v, want two parallel reads", stages[0])
	}
	if stages[1].Parallel || len(stages[1].Calls) != 1 || stages[1].Calls[0].Name != "action" {
		t.Fatalf("second stage=%#v, want sequential action barrier", stages[1])
	}
	if stages[2].Parallel || len(stages[2].Calls) != 1 || stages[2].Calls[0].ID != "4" {
		t.Fatalf("third stage=%#v, want single sequential read", stages[2])
	}
}

func TestBatchPlannerDoesNotParallelizeConflictingOrUnsafeReads(t *testing.T) {
	first := &fake{name: "first", d: desc(t, "first", contract.ReadOnly, []string{"messages"}, nil, true, 0, nil)}
	conflicting := &fake{name: "conflicting", d: desc(t, "conflicting", contract.ReadOnly, []string{"messages"}, []string{"messages"}, true, 0, nil)}
	nonIdempotent := &fake{name: "non_idempotent", d: desc(t, "non_idempotent", contract.ReadOnly, []string{"users"}, nil, false, 0, nil)}
	registry := tool.NewRegistry([]tool.Tool{first, conflicting, nonIdempotent}, []string{"first", "conflicting", "non_idempotent"})
	planner := BatchPlanner{Registry: registry}

	stages, err := planner.Plan(ctx(t), []Call{
		{ID: "1", Name: "first", Arguments: json.RawMessage(`{}`)},
		{ID: "2", Name: "conflicting", Arguments: json.RawMessage(`{}`)},
		{ID: "3", Name: "non_idempotent", Arguments: json.RawMessage(`{}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 3 {
		t.Fatalf("stages=%#v, want three sequential stages", stages)
	}
	for index, stage := range stages {
		if stage.Parallel || len(stage.Calls) != 1 {
			t.Fatalf("stage %d=%#v, want one sequential call", index, stage)
		}
	}
}

func TestBatchPlannerRejectsInvalidCallIdentityBeforePlanning(t *testing.T) {
	planner := BatchPlanner{Registry: tool.NewRegistry(nil, nil)}
	if _, err := planner.Plan(ctx(t), []Call{{ID: "same", Name: "unknown"}, {ID: "same", Name: "unknown"}}); err == nil {
		t.Fatal("duplicate IDs were accepted")
	}
}
