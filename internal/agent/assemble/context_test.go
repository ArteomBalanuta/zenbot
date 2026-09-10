package assemble

import (
	"encoding/json"
	"strings"
	"testing"

	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/turn"
)

func TestContextBudgeterKeepsCompleteUnitsAndValidJSON(t *testing.T) {
	recent := `{"rows":[{"name":"alice","message":"` + strings.Repeat("x", 400) + `"}]}`
	input := ContextInput{
		RequiredPrefix: []Message{llm.NewLlmMessage("system", "policy", nil, "")},
		Optional: []ContextUnit{{
			Source:    ContextRecentRoom,
			Timestamp: 20,
			Priority:  100,
			Messages:  []Message{llm.NewLlmMessage("user", recent, nil, "")},
		}},
		RequiredSuffix: []Message{llm.NewLlmMessage("user", "newest request", nil, "")},
		MaxTokens:      50,
	}
	projection, err := (ContextBudgeter{}).Project(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Messages) != 2 || projection.Messages[1].Content() != "newest request" {
		t.Fatalf("projection split or retained oversized unit: %#v", projection.Messages)
	}
	if !json.Valid([]byte(recent)) {
		t.Fatal("test fixture is not valid JSON")
	}
}

func TestContextBudgeterDeduplicatesAcrossSourcesAndKeepsHigherPriorityUnit(t *testing.T) {
	duplicate := llm.NewLlmMessage("user", "same evidence", nil, "")
	projection, err := (ContextBudgeter{}).Project(ContextInput{
		RequiredPrefix: []Message{llm.NewLlmMessage("system", "policy", nil, "")},
		Optional: []ContextUnit{
			{Source: ContextMemory, Timestamp: 10, Priority: 10, Messages: []Message{duplicate}},
			{Source: ContextRecentRoom, Timestamp: 20, Priority: 20, Messages: []Message{duplicate}},
			{Source: ContextRecentRoom, Timestamp: 30, Priority: 20, Messages: []Message{llm.NewLlmMessage("user", "newer", nil, "")}},
		},
		RequiredSuffix: []Message{llm.NewLlmMessage("user", "current", nil, "")},
		MaxTokens:      100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if projection.RemovedUnits != 1 || len(projection.Messages) != 4 {
		t.Fatalf("deduplicated projection=%#v", projection)
	}
	if projection.Messages[1].Content() != "same evidence" || projection.Messages[2].Content() != "newer" {
		t.Fatalf("source order was not deterministic: %#v", projection.Messages)
	}
}

func TestContextBudgeterSupportsOneMillionTokenCeiling(t *testing.T) {
	projection, err := (ContextBudgeter{}).Project(ContextInput{
		RequiredPrefix: []Message{llm.NewLlmMessage("system", "policy", nil, "")},
		RequiredSuffix: []Message{llm.NewLlmMessage("user", "current", nil, "")},
		MaxTokens:      1_000_000,
		ReserveTokens:  100_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if projection.BudgetChars != 3_600_000 {
		t.Fatalf("budget chars=%d, want 3600000", projection.BudgetChars)
	}
	if _, err := (ContextBudgeter{}).Project(ContextInput{MaxTokens: 1_000_001}); err == nil {
		t.Fatal("context ceiling above one million tokens was accepted")
	}
}

func TestContextBudgeterReservesProviderToolManifestCapacity(t *testing.T) {
	projection, err := (ContextBudgeter{}).Project(ContextInput{
		RequiredPrefix: []Message{llm.NewLlmMessage("system", "policy", nil, "")},
		RequiredSuffix: []Message{llm.NewLlmMessage("user", "current", nil, "")},
		MaxTokens:      100,
		ReserveTokens:  10,
		ManifestTokens: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if projection.BudgetChars != 240 {
		t.Fatalf("budget chars=%d, want 240", projection.BudgetChars)
	}
	if _, err := (ContextBudgeter{}).Project(ContextInput{MaxTokens: 20, ReserveTokens: 10, ManifestTokens: 10}); err == nil {
		t.Fatal("manifest exhausted context capacity without an error")
	}
}

func TestProjectTurnPreservesTaskAndAtomicBoundedObservation(t *testing.T) {
	objective := "inspect the current records"
	task, err := turn.NewTaskContract("request", objective, nil, []turn.Obligation{{ID: "answer", Kind: turn.ObligationAnswer, Required: true}})
	if err != nil {
		t.Fatal(err)
	}
	state := turn.NewTaskState(task)
	result := contract.SuccessResult("call-1", "lookup", map[string]any{"rows": []string{strings.Repeat("x", 500), strings.Repeat("y", 500)}})
	store := NewObservationStore()
	store.Store(result, 300)
	policy := llm.NewLlmMessage("system", "policy", nil, "")
	newest := llm.NewLlmMessage("user", objective, nil, "")
	call := llm.NewLlmToolCall("call-1", "lookup", map[string]any{})
	projection, err := ProjectTurn([]Message{
		policy,
		llm.NewLlmMessage("user", strings.Repeat("old", 1000), nil, ""),
		newest,
		llm.NewLlmMessage("assistant", nil, []llm.LlmToolCall{call}, ""),
		llm.NewLlmMessage("tool", string(result.Envelope()), nil, "call-1"),
	}, []any{map[string]any{"name": "lookup"}}, state, store, ContextInput{
		RequiredPrefix: []Message{policy}, RequiredSuffix: []Message{newest},
		MaxTokens: 350, ReserveTokens: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !projection.Pruned || len(projection.Messages) != 5 {
		t.Fatalf("projection did not drop only old context: %#v", projection)
	}
	if !strings.Contains(projection.Messages[1].Content(), task.RequestHash) || projection.Messages[2].Content() != objective {
		t.Fatalf("required task/request missing: %#v", projection.Messages)
	}
	if len(projection.Messages[3].ToolCalls()) != 1 || projection.Messages[4].ToolCallID() != "call-1" || !json.Valid([]byte(projection.Messages[4].Content())) {
		t.Fatalf("tool protocol was split or observation invalid: %#v", projection.Messages)
	}
	if len(projection.Messages[4].Content()) > 300 || projection.Messages[4].Content() == string(result.Envelope()) {
		t.Fatal("full observation leaked into projected transcript")
	}
}

func TestSplitTurnContextAnchorsNewestDuplicateAtLastOccurrence(t *testing.T) {
	policy := llm.NewLlmMessage("system", "policy", nil, "")
	request := llm.NewLlmMessage("user", "same request", nil, "")
	call := llm.NewLlmToolCall("call", "lookup", map[string]any{})
	before, after := splitTurnContext([]Message{
		policy,
		request,
		llm.NewLlmMessage("assistant", "old answer", nil, ""),
		request,
		llm.NewLlmMessage("assistant", nil, []llm.LlmToolCall{call}, ""),
		llm.NewLlmMessage("tool", `{"status":"success"}`, nil, "call"),
	}, []Message{policy}, []Message{request})
	if len(before) != 2 || len(after) != 1 || len(after[0].Messages) != 2 {
		t.Fatalf("duplicate newest request anchored incorrectly: before=%#v after=%#v", before, after)
	}
}
