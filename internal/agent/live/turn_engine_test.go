package live

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/llm"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/turn"
)

type engineActionTool struct {
	calls        atomic.Int32
	capabilities []string
	errorCode    string
	name         string
}

func (t *engineActionTool) Name() string {
	if t.name != "" {
		return t.name
	}
	return "engine_action"
}
func (t *engineActionTool) Descriptor(api.Context) (contract.Descriptor, error) {
	return contract.NewDescriptor(t.Name(), "Engine action", "Execute a state-changing test action.", "test",
		contract.AccessUser, contract.Action, contract.ModelData,
		json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		t.capabilities, nil, false, time.Second, json.RawMessage(`{"type":"object"}`), nil, []string{"room"}, []string{"Do not use for reads."})
}
func (t *engineActionTool) Execute(context.Context, api.Context, json.RawMessage) (contract.Result, error) {
	t.calls.Add(1)
	if t.errorCode != "" {
		return contract.ErrorResult("", t.Name(), t.errorCode, "action outcome is unknown"), nil
	}
	return contract.ActionSuccessResult("", t.Name(), map[string]any{"executed": true}, 0), nil
}

type failingModelClient struct{}

func (failingModelClient) Complete(context.Context, llm.LlmRequest) (llm.LlmResponse, error) {
	return llm.LlmResponse{}, errors.New("provider unavailable")
}
func configuredTurnEngine(t *testing.T, engine TurnEngine) TurnEngine {
	t.Helper()
	engine.Projector = testLiveAssembler(t)
	engine.NewestRequest = llm.NewLlmMessage("user", engine.Prompt, nil, "")
	engine.Observations = assemble.NewObservationStore()
	return engine
}
func engineMessages(prompt string) []llm.LlmMessage {
	return []llm.LlmMessage{llm.NewLlmMessage("system", "test policy", nil, ""), llm.NewLlmMessage("user", prompt, nil, "")}
}
func mustAgentContext(t *testing.T) api.Context {
	t.Helper()
	agent, err := api.NewContext("room", "caller", "", "", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	return agent
}
func TestTurnEnginePreservesCompletedEffectsWhenFollowingModelCallFails(t *testing.T) {
	action := &engineActionTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{action}, []string{action.Name()})
	limits := turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2}
	engine := configuredTurnEngine(t, TurnEngine{Client: failingModelClient{}, Registry: registry, Agent: mustAgentContext(t),
		Allowed: []string{action.Name()}, Limits: limits, Prompt: "perform the action and summarize"})
	definitions, err := providerToolDefinitions(registry, engine.Agent)
	if err != nil {
		t.Fatal(err)
	}
	state := turn.NewState(limits)
	state.AdvanceStep()
	initial := llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("action-1", action.Name(), map[string]any{})}, "tool_calls")
	_, messages, batch, err := engine.Complete(context.Background(), engineMessages(engine.Prompt), definitions, initial, state)
	if err == nil || len(batch) != 1 || !batch[0].Result.EffectsCommitted || action.calls.Load() != 1 {
		t.Fatalf("partial effects lost: batch=%#v calls=%d err=%v", batch, action.calls.Load(), err)
	}
	if !messagesContain(messages, "executed") {
		t.Fatal("completed observation disappeared from transcript")
	}
}
func TestTurnEngineCorrectsDuplicateIDWithoutOverwritingPriorObservation(t *testing.T) {
	read := &correctingReadTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{read}, []string{read.Name()})
	limits := turn.ExecutionLimits{MaxSteps: 5, MaxToolCalls: 4}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("same", read.Name(), map[string]any{"value": "second"})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("fresh", read.Name(), map[string]any{"value": "second"})}, "tool_calls"),
		llm.NewLlmResponse("Both observations are available.", nil, "stop"),
	}}
	engine := configuredTurnEngine(t, TurnEngine{Client: client, Registry: registry, Agent: mustAgentContext(t), Allowed: []string{read.Name()}, Limits: limits, Prompt: "read first then second"})
	definitions, _ := providerToolDefinitions(registry, engine.Agent)
	state := turn.NewState(limits)
	state.AdvanceStep()
	initial := llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("same", read.Name(), map[string]any{"value": "first"})}, "tool_calls")
	_, _, batch, err := engine.Complete(context.Background(), engineMessages(engine.Prompt), definitions, initial, state)
	if err != nil || len(batch) != 2 || read.calls.Load() != 2 {
		t.Fatalf("batch=%#v calls=%d err=%v", batch, read.calls.Load(), err)
	}
	original, ok := engine.Observations.Full("same")
	if !ok || original.CallID != "same" {
		t.Fatal("original observation overwritten")
	}
	if !messagesContain(client.requests[1].Messages(), "already used") {
		t.Fatal("fresh-ID correction was not explained")
	}
}
func TestTurnEngineMixedMalformedArgumentsPreserveSuccessfulSiblingForRepair(t *testing.T) {
	read := &correctingReadTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{read}, []string{read.Name()})
	limits := turn.ExecutionLimits{MaxSteps: 4, MaxToolCalls: 4, MaxCallsPerTool: 4}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("repaired", read.Name(), map[string]any{"value": "fixed"})}, "tool_calls"),
		llm.NewLlmResponse("Both reads succeeded.", nil, "stop"),
	}}
	engine := configuredTurnEngine(t, TurnEngine{Client: client, Registry: registry, Agent: mustAgentContext(t), Allowed: []string{read.Name()}, Limits: limits, Prompt: "obtain both values"})
	definitions, _ := providerToolDefinitions(registry, engine.Agent)
	state := turn.NewState(limits)
	state.AdvanceStep()
	initial := llm.NewLlmResponse(nil, []llm.LlmToolCall{
		llm.NewLlmToolCall("bad", read.Name(), "{broken"),
		llm.NewLlmToolCall("good", read.Name(), map[string]any{"value": "known"}),
	}, "tool_calls")
	_, _, batch, err := engine.Complete(context.Background(), engineMessages(engine.Prompt), definitions, initial, state)
	if err != nil || len(batch) != 3 || batch[0].Result.ErrorCode != "INVALID_ARGUMENTS" || batch[1].Result.IsError || batch[2].Result.IsError {
		t.Fatalf("batch=%#v err=%v", batch, err)
	}
	if !providerRequestHasTool(client.requests[0], read.Name()) || !messagesContain(client.requests[0].Messages(), "INVALID_ARGUMENTS") {
		t.Fatal("model lost repair tools or error feedback")
	}
	assertAtomicToolProtocol(t, 0, client.requests[0].Messages())
}
func TestTurnEngineThreeDependentRoundsRetainRequestAndEveryObservation(t *testing.T) {
	read := &correctingReadTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{read}, []string{read.Name()})
	limits := turn.ExecutionLimits{MaxSteps: 5, MaxToolCalls: 4, MaxCallsPerTool: 4}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("two", read.Name(), map[string]any{"value": "second"})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("three", read.Name(), map[string]any{"value": "third"})}, "tool_calls"),
		llm.NewLlmResponse("Combined first, second and third.", nil, "stop"),
	}}
	objective := "Read the first value, use it for the second, inspect the third and give one combined answer."
	engine := configuredTurnEngine(t, TurnEngine{Client: client, Registry: registry, Agent: mustAgentContext(t), Allowed: []string{read.Name()}, Limits: limits, Prompt: objective})
	definitions, _ := providerToolDefinitions(registry, engine.Agent)
	state := turn.NewState(limits)
	state.AdvanceStep()
	initial := llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("one", read.Name(), map[string]any{"value": "first"})}, "tool_calls")
	response, _, batch, err := engine.Complete(context.Background(), engineMessages(objective), definitions, initial, state)
	if err != nil || len(batch) != 3 || response.Content() != "Combined first, second and third." {
		t.Fatalf("response=%q batch=%d err=%v", response.Content(), len(batch), err)
	}
	for i, request := range client.requests {
		if !messagesContain(request.Messages(), objective) {
			t.Fatalf("round %d lost original objective", i)
		}
		for _, id := range []string{"one", "two", "three"}[:i+1] {
			if !requestContainsToolCallID(request, id) {
				t.Fatalf("round %d lost observation %s", i, id)
			}
		}
	}
}
func TestTurnEngineBudgetsBecomeExplicitTerminalInstructions(t *testing.T) {
	read := &correctingReadTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{read}, []string{read.Name()})
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("Read completed; further work could not fit.", nil, "stop")}}
	limits := turn.ExecutionLimits{MaxSteps: 1, MaxToolCalls: 1}
	engine := configuredTurnEngine(t, TurnEngine{Client: client, Registry: registry, Agent: mustAgentContext(t), Allowed: []string{read.Name()}, Limits: limits, Prompt: "read then explain"})
	definitions, _ := providerToolDefinitions(registry, engine.Agent)
	state := turn.NewState(limits)
	state.AdvanceStep()
	initial := llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("last", read.Name(), map[string]any{"value": "current"})}, "tool_calls")
	_, _, _, err := engine.Complete(context.Background(), engineMessages(engine.Prompt), definitions, initial, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 || len(client.requests[0].Tools()) != 0 || !messagesContain(client.requests[0].Messages(), `"terminal":true`) {
		t.Fatalf("terminal context=%#v", client.requests)
	}
}
func TestTurnEngineDoesNotExecuteTruncatedActionCalls(t *testing.T) {
	action := &engineActionTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{action}, []string{action.Name()})
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("No action ran.", nil, "stop")}}
	limits := turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 1}
	engine := configuredTurnEngine(t, TurnEngine{Client: client, Registry: registry, Agent: mustAgentContext(t), Allowed: []string{action.Name()}, Limits: limits, Prompt: "do it"})
	defs, _ := providerToolDefinitions(registry, engine.Agent)
	state := turn.NewState(limits)
	state.AdvanceStep()
	initial := llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("cut", action.Name(), map[string]any{})}, "length")
	_, _, batch, err := engine.Complete(context.Background(), engineMessages(engine.Prompt), defs, initial, state)
	if err != nil || action.calls.Load() != 0 || len(batch) != 0 {
		t.Fatalf("calls=%d batch=%d err=%v", action.calls.Load(), len(batch), err)
	}
	if !strings.Contains(client.requests[0].Messages()[len(client.requests[0].Messages())-2].Content(), "No calls") && !messagesContain(client.requests[0].Messages(), "No calls") {
		t.Fatal("truncated call was not explained")
	}
}

func TestTurnEngineUnknownActionHidesActionsButAllowsVerificationReads(t *testing.T) {
	action := &engineActionTool{errorCode: "ACTION_OUTCOME_UNKNOWN"}
	read := &correctingReadTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{action, read}, []string{action.Name(), read.Name()})
	limits := turn.ExecutionLimits{MaxSteps: 4, MaxToolCalls: 5}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("verify", read.Name(), map[string]any{"value": "current state"})}, "tool_calls"),
		llm.NewLlmResponse("The action outcome is uncertain; I checked the current state without repeating it.", nil, "stop"),
	}}
	engine := configuredTurnEngine(t, TurnEngine{Client: client, Registry: registry, Agent: mustAgentContext(t),
		Allowed: []string{action.Name(), read.Name()}, Limits: limits, Prompt: "perform the action and verify it"})
	defs, _ := providerToolDefinitions(registry, engine.Agent)
	state := turn.NewState(limits)
	state.AdvanceStep()
	initial := llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("uncertain", action.Name(), map[string]any{})}, "tool_calls")
	_, _, batch, err := engine.Complete(context.Background(), engineMessages(engine.Prompt), defs, initial, state)
	if err != nil || len(batch) != 2 || action.calls.Load() != 1 || read.calls.Load() != 1 {
		t.Fatalf("batch=%#v err=%v", batch, err)
	}
	for _, request := range client.requests {
		if providerRequestHasTool(request, action.Name()) || !providerRequestHasTool(request, read.Name()) {
			t.Fatal("unknown action must hide effecting tools without disabling verification reads")
		}
		if !messagesContain(request.Messages(), `"blockedByUnknownAction":"uncertain"`) {
			t.Fatal("model lacks current action-block reason")
		}
	}
}

func TestTurnEngineManifestRejectionPreservesFailedActionBarrier(t *testing.T) {
	unexposed := &engineActionTool{name: "unexposed_action"}
	allowed := &engineActionTool{name: "allowed_action"}
	read := &correctingReadTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{unexposed, allowed, read}, []string{unexposed.Name(), allowed.Name(), read.Name()})
	limits := turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 4}
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("The first action was unavailable and the following action was skipped; the read succeeded.", nil, "stop")}}
	engine := configuredTurnEngine(t, TurnEngine{Client: client, Registry: registry, Agent: mustAgentContext(t), Allowed: []string{allowed.Name(), read.Name()}, Limits: limits, Prompt: "execute the requested work"})
	descriptors, _ := providerToolDefinitions(registry, engine.Agent)
	var exposed []any
	for _, definition := range descriptors {
		if !providerToolNames([]any{definition})[unexposed.Name()] {
			exposed = append(exposed, definition)
		}
	}
	state := turn.NewState(limits)
	state.AdvanceStep()
	initial := llm.NewLlmResponse(nil, []llm.LlmToolCall{
		llm.NewLlmToolCall("hidden", unexposed.Name(), map[string]any{}),
		llm.NewLlmToolCall("later", allowed.Name(), map[string]any{}),
		llm.NewLlmToolCall("read", read.Name(), map[string]any{"value": "current"}),
	}, "tool_calls")
	_, _, batch, err := engine.Complete(context.Background(), engineMessages(engine.Prompt), exposed, initial, state)
	if err != nil || len(batch) != 3 || batch[0].Result.ErrorCode != "TOOL_NOT_ALLOWED" || batch[1].Result.ErrorCode != "ACTION_NOT_EXECUTED" || batch[1].Result.RelatedCallID != "hidden" || batch[2].Result.IsError {
		t.Fatalf("batch=%#v err=%v", batch, err)
	}
	if unexposed.calls.Load() != 0 || allowed.calls.Load() != 0 || read.calls.Load() != 1 {
		t.Fatal("manifest rejection lost the ordered action barrier or independent read")
	}
}

func TestTurnEngineRetainsWholeExecutedBatchWhenObservationMetadataCannotFit(t *testing.T) {
	read := &correctingReadTool{}
	action := &engineActionTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{read, action}, []string{read.Name(), action.Name()})
	limits := turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2}
	engine := configuredTurnEngine(t, TurnEngine{Client: &scriptedToolClient{}, Registry: registry, Agent: mustAgentContext(t),
		Allowed: []string{read.Name(), action.Name()}, Limits: limits, Prompt: "read then act"})
	definitions, _ := providerToolDefinitions(registry, engine.Agent)
	state := turn.NewState(limits)
	state.AdvanceStep()
	longID := strings.Repeat("r", contract.DefaultMaxModelResultBytes+1)
	initial := llm.NewLlmResponse(nil, []llm.LlmToolCall{
		llm.NewLlmToolCall(longID, read.Name(), map[string]any{"value": "known"}),
		llm.NewLlmToolCall("committed", action.Name(), map[string]any{}),
	}, "tool_calls")
	original := engineMessages(engine.Prompt)
	_, messages, batch, err := engine.Complete(context.Background(), original, definitions, initial, state)
	if err == nil || len(batch) != 2 || action.calls.Load() != 1 {
		t.Fatalf("batch=%#v err=%v", batch, err)
	}
	if len(engine.Observations.Index(0)) != 2 {
		t.Fatal("projection failure lost a later executed result")
	}
	result, found := engine.Observations.Full("committed")
	if !found || !result.EffectsCommitted {
		t.Fatal("committed effect missing from retained observations")
	}
	if len(messages) != len(original) || !messagesContain(messages, engine.Prompt) {
		t.Fatal("failed protocol projection overwrote the valid conversation")
	}
}
