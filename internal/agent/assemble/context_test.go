package assemble

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool/contract"
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

func TestThreeRoundsKeepOriginalRequestAndReceiptsWhenResultsArePruned(t *testing.T) {
	store := NewObservationStore()
	policy := llm.NewLlmMessage("system", "policy", nil, "")
	newest := llm.NewLlmMessage("user", "Look up Alice, notify her, then check the weather and combine the outcomes.", nil, "")
	messages := []Message{policy, newest}
	for round, name := range []string{"lookup", "notify", "weather"} {
		id := fmt.Sprintf("call-%d", round)
		args := json.RawMessage(`{"target":"alice"}`)
		store.RecordCall(id, name, args)
		result := contract.SuccessResult(id, name, map[string]any{"value": strings.Repeat(name, 500)})
		if name == "notify" {
			result.EffectsCommitted, result.DeliveryCount = true, 1
		}
		view := store.Store(result, 2400)
		messages = append(messages, llm.NewLlmMessage("assistant", nil, []llm.LlmToolCall{llm.NewLlmToolCall(id, name, string(args))}, ""), llm.NewLlmMessage("tool", string(view.JSON()), nil, id))
	}
	projection, err := ProjectTurn(messages, nil, store, ContextInput{RequiredPrefix: []Message{policy}, RequiredSuffix: []Message{newest}, MaxTokens: 350})
	if err != nil {
		t.Fatal(err)
	}
	if !projection.Pruned || projection.SerializedChars > projection.BudgetChars {
		t.Fatalf("projection did not honor the forced budget: %#v", projection)
	}
	var index struct {
		Results []struct {
			CallID, Tool, Status string
			Arguments            json.RawMessage
			EffectsCommitted     bool
			DeliveryCount        int
		}
		OmittedContextUnits int
	}
	foundRequest, foundIndex := false, false
	for _, message := range projection.Messages {
		if message.Content() == newest.Content() {
			foundRequest = true
		}
		if strings.HasPrefix(message.Content(), "CURRENT_TURN_RESULTS_UNTRUSTED_DATA=") {
			foundIndex = true
			if err := json.Unmarshal([]byte(strings.TrimPrefix(message.Content(), "CURRENT_TURN_RESULTS_UNTRUSTED_DATA=")), &index); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !foundRequest || !foundIndex || len(index.Results) != 3 || index.OmittedContextUnits == 0 {
		t.Fatalf("request or execution evidence was lost: %#v", projection.Messages)
	}
	if index.Results[0].CallID != "call-0" || index.Results[0].Tool != "lookup" || string(index.Results[0].Arguments) != `{"target":"alice"}` {
		t.Fatalf("early call identity or target was lost: %#v", index)
	}
	if !index.Results[1].EffectsCommitted || index.Results[1].DeliveryCount != 1 {
		t.Fatalf("committed action receipt was lost: %#v", index.Results[1])
	}
	full, found := store.Full("call-0")
	if !found || !strings.Contains(full.Content, strings.Repeat("lookup", 500)) {
		t.Fatal("pruned original result is no longer retrievable")
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

func TestProjectTurnPreservesNewestRequestAndAtomicBoundedObservation(t *testing.T) {
	objective := "inspect the current records"
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
	}, []any{map[string]any{"name": "lookup"}}, store, ContextInput{
		RequiredPrefix: []Message{policy}, RequiredSuffix: []Message{newest},
		MaxTokens: 350, ReserveTokens: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !projection.Pruned || len(projection.Messages) != 5 {
		t.Fatalf("projection did not drop only old context: %#v", projection)
	}
	if projection.Messages[1].Content() != objective {
		t.Fatalf("required newest request missing: %#v", projection.Messages)
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

func TestRequiredResultIndexShrinksArgumentsBeforeLosingUncertainReceipt(t *testing.T) {
	store := NewObservationStore()
	arguments := json.RawMessage(`{"body":"` + strings.Repeat("payload", 500) + `"}`)
	store.RecordCall("uncertain", "notify", arguments)
	result := contract.ActionErrorResult("uncertain", "notify", "ACTION_OUTCOME_UNKNOWN", "Connection ended after submission.", contract.EffectUnknown)
	store.Store(result, 512)
	policy := llm.NewLlmMessage("system", "policy", nil, "")
	request := llm.NewLlmMessage("user", "Notify Alice and report the outcome.", nil, "")
	projection, err := ProjectTurn([]Message{policy, request}, nil, store, ContextInput{RequiredPrefix: []Message{policy}, RequiredSuffix: []Message{request}, MaxTokens: 180})
	if err != nil {
		t.Fatal(err)
	}
	if projection.SerializedChars > projection.BudgetChars {
		t.Fatalf("receipt index exceeded context: %#v", projection)
	}
	var payload resultIndexPayload
	for _, message := range projection.Messages {
		if strings.HasPrefix(message.Content(), resultIndexPrefix) {
			if err := json.Unmarshal([]byte(strings.TrimPrefix(message.Content(), resultIndexPrefix)), &payload); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(payload.Results) != 1 || payload.Results[0].CallID != "uncertain" || payload.Results[0].EffectState != contract.EffectUnknown || !payload.Results[0].ArgumentsTruncated {
		t.Fatalf("argument preview displaced an uncertain action receipt: %#v", payload)
	}
	retained, found := store.CallArguments("uncertain")
	if !found || string(retained) != string(arguments) {
		t.Fatal("shrinking preview destroyed original arguments")
	}
}

func TestCurrentRuntimeStatusSurvivesPruningAndRemainsLast(t *testing.T) {
	policy := llm.NewLlmMessage("system", "policy", nil, "")
	request := llm.NewLlmMessage("user", "Finish the original request.", nil, "")
	status := llm.NewLlmMessage("user", `TOOL_LOOP_STATUS={"terminal":true,"remainingToolCalls":0}`, nil, "")
	store := NewObservationStore()
	store.Store(contract.ActionSuccessResult("sent", "notify", map[string]any{"value": strings.Repeat("x", 2000)}, 1), 512)
	messages := []Message{policy, request,
		llm.NewLlmMessage("assistant", nil, []llm.LlmToolCall{llm.NewLlmToolCall("sent", "notify", map[string]any{})}, ""),
		llm.NewLlmMessage("tool", string(store.Views()[0].JSON()), nil, "sent"), status}
	projection, err := ProjectTurn(messages, nil, store, ContextInput{RequiredPrefix: []Message{policy}, RequiredSuffix: []Message{request}, RequiredRuntime: []Message{status}, MaxTokens: 225})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, message := range projection.Messages {
		if message.Content() == status.Content() {
			count++
		}
	}
	if count != 1 || projection.Messages[len(projection.Messages)-1].Content() != status.Content() || !projection.Pruned || projection.SerializedChars > projection.BudgetChars {
		t.Fatalf("current runtime control was removed, duplicated, reordered or overflowed: %#v", projection)
	}
}
