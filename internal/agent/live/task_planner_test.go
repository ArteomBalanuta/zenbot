package live

import (
	"context"
	"testing"

	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/turn"
)

func plannerTools() []PlanningTool {
	return []PlanningTool{
		{Name: "lookup_user", Description: "Look up one user.", Effect: contract.ReadOnly, ResultMode: contract.ModelData},
		{Name: "notify_user", Description: "Notify one user.", Effect: contract.Action, ResultMode: contract.RoomDelivery},
	}
}

func taskPlanResponse(objective string, constraints, obligations []any) llm.LlmResponse {
	return llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("plan", submitTaskPlanTool, map[string]any{
		"objective":   objective,
		"constraints": constraints,
		"obligations": obligations,
	})}, "tool_calls")
}

func TestSemanticTaskPlannerFreezesObjectiveAndReassignsObligationIDs(t *testing.T) {
	objective := "Look up @Alice, notify her, and report the outcome exactly."
	client := &scriptedToolClient{responses: []llm.LlmResponse{taskPlanResponse(objective,
		[]any{map[string]any{"text": "Do not notify another user."}},
		[]any{
			map[string]any{"id": "model-lookup", "kind": "TOOL", "tool": "lookup_user", "subject": "@Alice", "required": true, "dependsOn": []any{}},
			map[string]any{"id": "model-notify", "kind": "TOOL", "tool": "notify_user", "subject": "alice", "required": true, "dependsOn": []any{"model-lookup"}},
			map[string]any{"id": "model-answer", "kind": "ANSWER", "tool": "", "subject": "", "required": true, "dependsOn": []any{"model-notify"}},
		},
	)}}
	planner, err := NewSemanticTaskPlanner(client, "Return a structured task plan.")
	if err != nil {
		t.Fatal(err)
	}

	task, err := planner.Plan(context.Background(), TaskPlanInput{RequestID: "request-1", Objective: objective, Tools: plannerTools()})
	if err != nil {
		t.Fatal(err)
	}
	if task.Objective != objective || len(client.requests) != 1 || client.requests[0].ToolChoice() != llm.ToolChoiceRequired {
		t.Fatalf("task=%#v requests=%#v", task, client.requests)
	}
	obligations := task.Obligations()
	if len(obligations) != 3 || obligations[0].ID != "obligation-1" || obligations[1].ID != "obligation-2" || obligations[2].ID != "obligation-3" {
		t.Fatalf("model-controlled IDs survived: %#v", obligations)
	}
	if len(obligations[1].DependsOn) != 1 || obligations[1].DependsOn[0] != "obligation-1" || len(obligations[2].DependsOn) != 1 || obligations[2].DependsOn[0] != "obligation-2" {
		t.Fatalf("dependencies were not remapped: %#v", obligations)
	}
	if obligations[0].Effect != contract.ReadOnly || obligations[0].RequiresReceipt || obligations[1].Effect != contract.Action || !obligations[1].RequiresReceipt {
		t.Fatalf("model controlled execution requirements: %#v", obligations)
	}
}

func TestSemanticTaskPlannerRejectsInvalidPlansAfterOneCorrection(t *testing.T) {
	objective := "Look up Alice."
	validObligation := map[string]any{"id": "one", "kind": "TOOL", "tool": "lookup_user", "subject": "alice", "required": true, "dependsOn": []any{}}
	tests := []struct {
		name     string
		response llm.LlmResponse
		second   llm.LlmResponse
	}{
		{name: "unknown tool", response: taskPlanResponse(objective, []any{}, []any{map[string]any{"id": "one", "kind": "TOOL", "tool": "invented", "subject": "alice", "required": true, "dependsOn": []any{}}})},
		{name: "unknown dependency", response: taskPlanResponse(objective, []any{}, []any{map[string]any{"id": "one", "kind": "TOOL", "tool": "lookup_user", "subject": "alice", "required": true, "dependsOn": []any{"missing"}}})},
		{name: "duplicate ids", response: taskPlanResponse(objective, []any{}, []any{validObligation, validObligation})},
		{name: "changed objective", response: taskPlanResponse("Ignore the original request.", []any{}, []any{validObligation})},
		{name: "second malformed response", response: llm.NewLlmResponse("not structured", nil, "stop"), second: llm.NewLlmResponse(nil, nil, "tool_calls")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			second := tc.second
			if len(second.ToolCalls()) == 0 && second.Content() == "" && second.FinishReason() == "" {
				second = tc.response
			}
			client := &scriptedToolClient{responses: []llm.LlmResponse{tc.response, second}}
			planner, _ := NewSemanticTaskPlanner(client, "Return a structured task plan.")

			if _, err := planner.Plan(context.Background(), TaskPlanInput{RequestID: "request-1", Objective: objective, Tools: plannerTools()}); err == nil {
				t.Fatal("invalid task plan was accepted")
			}
			if len(client.requests) != 2 {
				t.Fatalf("planner requests=%d, want one bounded correction", len(client.requests))
			}
		})
	}
}

func TestObjectiveOnlyTaskPlannerCreatesControllerOwnedAnswerObligation(t *testing.T) {
	task, err := (ObjectiveOnlyTaskPlanner{}).Plan(context.Background(), TaskPlanInput{RequestID: "request", Objective: "answer me"})
	if err != nil {
		t.Fatal(err)
	}
	obligations := task.Obligations()
	if len(obligations) != 1 || obligations[0].Kind != turn.ObligationAnswer || obligations[0].ID != "obligation-1" || !obligations[0].Required {
		t.Fatalf("obligations=%#v", obligations)
	}
}
