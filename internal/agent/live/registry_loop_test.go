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
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/runtime"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
	"zenbot/internal/agent/turn"
)

type parallelReadTool struct {
	name    string
	started *atomic.Int32
	ready   chan struct{}
}

type correctingReadTool struct{ calls atomic.Int32 }

type failingReadTool struct{ calls atomic.Int32 }

type invalidDescriptorTool struct{}

type roomDeliveryTool struct {
	name   string
	result contract.Result
}

func (t roomDeliveryTool) Name() string { return t.name }
func (t roomDeliveryTool) Descriptor(api.Context) (contract.Descriptor, error) {
	return contract.NewDescriptor(t.name, t.name, "Deliver one command result directly to the room.", "test", contract.AccessUser, contract.Action, contract.RoomDelivery, json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), nil, nil, false, time.Second, json.RawMessage(`{"type":"object"}`), nil, []string{"room_delivery"}, []string{"Do not use for model-only data."})
}
func (t roomDeliveryTool) Execute(context.Context, api.Context, json.RawMessage) (contract.Result, error) {
	return t.result, nil
}

func (invalidDescriptorTool) Name() string { return "invalid_descriptor" }
func (invalidDescriptorTool) Descriptor(api.Context) (contract.Descriptor, error) {
	return contract.Descriptor{}, errors.New("broken descriptor")
}
func (invalidDescriptorTool) Execute(context.Context, api.Context, json.RawMessage) (contract.Result, error) {
	return contract.SuccessResult("", "invalid_descriptor", map[string]any{}), nil
}

func (t *correctingReadTool) Name() string { return "correcting_read" }
func (t *correctingReadTool) Descriptor(api.Context) (contract.Descriptor, error) {
	return contract.NewDescriptor(t.Name(), "Correcting read", "Read one validated value after correcting invalid arguments.", "test", contract.AccessUser, contract.ReadOnly, contract.ModelData, json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","minLength":1}},"required":["value"]}`), nil, nil, true, time.Second, json.RawMessage(`{"type":"object"}`), []string{"messages"}, nil, []string{"Do not call with a blank value."})
}
func (t *correctingReadTool) Execute(_ context.Context, _ api.Context, _ json.RawMessage) (contract.Result, error) {
	t.calls.Add(1)
	return contract.SuccessResult("", t.Name(), map[string]any{"value": "observed"}), nil
}

func (t *failingReadTool) Name() string { return "failing_read" }
func (t *failingReadTool) Descriptor(api.Context) (contract.Descriptor, error) {
	return contract.NewDescriptor(t.Name(), "Failing read", "Fail predictably for request-local failure-budget verification.", "test", contract.AccessUser, contract.ReadOnly, contract.ModelData, json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string"}},"required":["value"]}`), nil, nil, true, 0, json.RawMessage(`{"type":"object"}`), []string{"messages"}, nil, []string{"Do not use outside tests."})
}
func (t *failingReadTool) Execute(_ context.Context, _ api.Context, _ json.RawMessage) (contract.Result, error) {
	t.calls.Add(1)
	return contract.Result{}, errors.New("expected tool failure")
}

func (t parallelReadTool) Name() string { return t.name }
func (t parallelReadTool) Descriptor(api.Context) (contract.Descriptor, error) {
	return contract.NewDescriptor(t.name, t.name, "Independent read for parallel registry-loop verification.", "test", contract.AccessUser, contract.ReadOnly, contract.ModelData, json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), nil, nil, true, 250*time.Millisecond, json.RawMessage(`{"type":"object"}`), []string{"messages"}, nil, []string{"Do not use for writes."})
}
func (t parallelReadTool) Execute(ctx context.Context, _ api.Context, _ json.RawMessage) (contract.Result, error) {
	if t.started.Add(1) == 2 {
		close(t.ready)
	}
	select {
	case <-t.ready:
		return contract.SuccessResult("", t.name, map[string]any{"value": "ok"}), nil
	case <-ctx.Done():
		return contract.Result{}, ctx.Err()
	}
}

func TestRegistryToolLoopRecordsReadEvidenceAndAttempt(t *testing.T) {
	directory := &loopRoomDirectory{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("room-call", roomUsersTool, map[string]any{})}, "tool_calls"),
		llm.NewLlmResponse("answer from evidence", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{agenttool.RoomUsers{Directory: directory}}, []string{roomUsersTool}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("registry-evidence", runtime.NewContext("room", "caller", "", "", false, nil), "hello there", runtime.AMBIENT, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if completion.Response.Content() != "answer from evidence" || !completion.ToolAttempted || directory.calls != 1 || len(completion.Evidence()) != 1 || completion.Evidence()[0].Tool != roomUsersTool || len(client.requests) != 2 || client.requests[1].Messages()[len(client.requests[1].Messages())-2].ToolCalls()[0].ID() != "room-call" {
		t.Fatalf("completion=%#v roomCalls=%d", completion, directory.calls)
	}
}

func TestRegistryToolLoopSuppressesFinalReplyAfterSuccessfulRoomDelivery(t *testing.T) {
	delivery := roomDeliveryTool{name: "delivered_command", result: contract.SuccessResult("", "delivered_command", map[string]any{"messages": []string{"already visible"}, "deliveredCount": 1})}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("delivery-call", delivery.Name(), map[string]any{})}, "tool_calls"),
		llm.NewLlmResponse("already visible", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{delivery}, []string{delivery.Name()}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("registry-delivery", runtime.NewContext("room", "caller", "", "", false, nil), "run it", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || !completion.SuppressReply || len(client.requests) != 2 {
		t.Fatalf("completion=%#v requests=%d err=%v", completion, len(client.requests), err)
	}
}

func TestRegistryToolLoopKeepsSynthesisForMixedOrFailedDeliveryResults(t *testing.T) {
	tests := []struct {
		name  string
		tools []agenttool.Tool
		calls []llm.LlmToolCall
	}{
		{
			name: "mixed model data",
			tools: []agenttool.Tool{
				roomDeliveryTool{name: "delivered_command", result: contract.SuccessResult("", "delivered_command", map[string]any{"deliveredCount": 1})},
				&correctingReadTool{},
			},
			calls: []llm.LlmToolCall{
				llm.NewLlmToolCall("delivery-call", "delivered_command", map[string]any{}),
				llm.NewLlmToolCall("read-call", "correcting_read", map[string]any{"value": "yes"}),
			},
		},
		{
			name: "failed delivery",
			tools: []agenttool.Tool{
				roomDeliveryTool{name: "delivered_command", result: contract.ErrorResult("", "delivered_command", "COMMAND_REJECTED", "not delivered")},
			},
			calls: []llm.LlmToolCall{llm.NewLlmToolCall("delivery-call", "delivered_command", map[string]any{})},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			allowed := make([]string, 0, len(tc.tools))
			for _, registered := range tc.tools {
				allowed = append(allowed, registered.Name())
			}
			client := &scriptedToolClient{responses: []llm.LlmResponse{
				llm.NewLlmResponse(nil, tc.calls, "tool_calls"),
				llm.NewLlmResponse("synthesis required", nil, "stop"),
			}}
			loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, tc.tools, allowed, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
			if err != nil {
				t.Fatal(err)
			}
			inv := runtime.NewInvocation("registry-delivery-control", runtime.NewContext("room", "caller", "", "", false, nil), "run it", runtime.MENTION, "", false)
			completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
			if err != nil || completion.SuppressReply || completion.Response.Content() != "synthesis required" {
				t.Fatalf("completion=%#v err=%v", completion, err)
			}
		})
	}
}

func TestRegistryToolLoopRejectsInvalidRestrictedDescriptorAtComposition(t *testing.T) {
	_, err := NewRegistryToolLoop(testLiveAssembler(t), &scriptedToolClient{}, []agenttool.Tool{invalidDescriptorTool{}}, []string{"invalid_descriptor"}, turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 1})
	if err == nil || !strings.Contains(err.Error(), "descriptor") {
		t.Fatalf("error=%v", err)
	}
}

func TestExecuteRegistryBatchFansOutIndependentReadOnlyCalls(t *testing.T) {
	var started atomic.Int32
	ready := make(chan struct{})
	tools := []agenttool.Tool{
		parallelReadTool{name: "read_one", started: &started, ready: ready},
		parallelReadTool{name: "read_two", started: &started, ready: ready},
	}
	registry := agenttool.NewRegistry(tools, []string{"read_one", "read_two"})
	executor := &execution.Executor{Registry: registry, Ledger: execution.NewLedger(map[string]int{"read_one": 1, "read_two": 1}, 2)}
	agent, _ := api.NewContext("room", "caller", "", "", false, []string{})
	state := turn.NewState(turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	_, batch, err := executeRegistryBatch(context.Background(), executor, agent, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2}, state, []execution.Call{
		{ID: "one", Name: "read_one", Arguments: json.RawMessage(`{}`)},
		{ID: "two", Name: "read_two", Arguments: json.RawMessage(`{}`)},
	})
	if err != nil || len(batch) != 2 || batch[0].Result.IsError || batch[1].Result.IsError || started.Load() != 2 {
		t.Fatalf("batch=%#v started=%d err=%v", batch, started.Load(), err)
	}
}

func TestExecuteRegistryBatchRejectsDuplicateCallIDsBeforeExecution(t *testing.T) {
	read := &correctingReadTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{read}, []string{read.Name()})
	executor := &execution.Executor{Registry: registry}
	agent, _ := api.NewContext("room", "caller", "", "", false, []string{})
	state := turn.NewState(turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	_, _, err := executeRegistryBatch(context.Background(), executor, agent, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2}, state, []execution.Call{
		{ID: "same", Name: read.Name(), Arguments: json.RawMessage(`{"value":"one"}`)},
		{ID: "same", Name: read.Name(), Arguments: json.RawMessage(`{"value":"two"}`)},
	})
	if err == nil {
		t.Fatal("duplicate call IDs were accepted")
	}
	if read.calls.Load() != 0 {
		t.Fatalf("tool executed %d times before identity validation", read.calls.Load())
	}
}

func TestRegistryToolLoopKeepsToolsAvailableForArgumentSelfCorrection(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("bad", read.Name(), map[string]any{"value": 7})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("fixed", read.Name(), map[string]any{"value": "yes"})}, "tool_calls"),
		llm.NewLlmResponse("corrected answer", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("self-correction", runtime.NewContext("room", "caller", "", "", false, nil), "read it", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || completion.Response.Content() != "corrected answer" || read.calls.Load() != 1 || len(client.requests) != 3 {
		t.Fatalf("completion=%#v calls=%d requests=%d err=%v", completion, read.calls.Load(), len(client.requests), err)
	}
	if len(client.requests[1].Tools()) != 1 || len(client.requests[2].Tools()) != 1 {
		t.Fatalf("tool manifest disappeared during correction: second=%d third=%d", len(client.requests[1].Tools()), len(client.requests[2].Tools()))
	}
	secondMessages := client.requests[1].Messages()
	if secondMessages[len(secondMessages)-1].Role() != "tool" || !strings.Contains(secondMessages[len(secondMessages)-1].Content(), "INVALID_ARGUMENTS") {
		t.Fatalf("invalid-argument observation missing: %#v", secondMessages)
	}
}

func TestRegistryToolLoopHonorsPerToolCallBudget(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("one", read.Name(), map[string]any{"value": "first"})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("two", read.Name(), map[string]any{"value": "second"})}, "tool_calls"),
		llm.NewLlmResponse("bounded answer", nil, "stop"),
	}}
	limits := turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 3, MaxCallsPerTool: 1}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, limits)
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("per-tool-budget", runtime.NewContext("room", "caller", "", "", false, nil), "read twice", runtime.MENTION, "", false)
	if _, err := loop.CompleteWithEvidence(context.Background(), inv, nil, ""); err != nil {
		t.Fatal(err)
	}
	if read.calls.Load() != 1 {
		t.Fatalf("tool executions=%d, want 1", read.calls.Load())
	}
	messages := client.requests[2].Messages()
	if !strings.Contains(messages[len(messages)-1].Content(), "TOOL_CALL_LIMIT_REACHED") {
		t.Fatalf("limit observation missing: %#v", messages[len(messages)-1])
	}
}

func TestRegistryToolLoopHonorsConfiguredFailureBudget(t *testing.T) {
	read := &failingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("one", read.Name(), map[string]any{"value": "first"})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("two", read.Name(), map[string]any{"value": "second"})}, "tool_calls"),
		llm.NewLlmResponse("degraded answer", nil, "stop"),
	}}
	limits := turn.ExecutionLimits{MaxSteps: 3, MaxToolCalls: 3, MaxToolFailures: 1}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, limits)
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("failure-budget", runtime.NewContext("room", "caller", "", "", false, nil), "read twice", runtime.MENTION, "", false)
	if _, err := loop.CompleteWithEvidence(context.Background(), inv, nil, ""); err != nil {
		t.Fatal(err)
	}
	if read.calls.Load() != 1 {
		t.Fatalf("tool executions=%d, want 1", read.calls.Load())
	}
	messages := client.requests[2].Messages()
	if !strings.Contains(messages[len(messages)-1].Content(), "TOOL_DISABLED") {
		t.Fatalf("disabled observation missing: %#v", messages[len(messages)-1])
	}
	if len(client.requests[2].Tools()) != 0 {
		t.Fatalf("disabled tool remained available during degradation: %#v", client.requests[2].Tools())
	}
}

func TestRegistryToolLoopReservesTerminalSynthesisAfterLastToolRound(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("one", read.Name(), map[string]any{"value": "first"})}, "tool_calls"),
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("two", read.Name(), map[string]any{"value": "second"})}, "tool_calls"),
		llm.NewLlmResponse("answer from both observations", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 2, MaxCallsPerTool: 2})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("terminal-reserve", runtime.NewContext("room", "caller", "", "", false, nil), "read twice", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil {
		t.Fatalf("last-round observations were discarded: %v", err)
	}
	if completion.Response.Content() != "answer from both observations" || read.calls.Load() != 2 || len(client.requests) != 3 {
		t.Fatalf("completion=%#v tool_calls=%d provider_calls=%d", completion, read.calls.Load(), len(client.requests))
	}
	if len(client.requests[2].Tools()) != 0 {
		t.Fatalf("terminal synthesis retained tools: %#v", client.requests[2].Tools())
	}
}

func TestRegistryToolLoopReflectsOnceOnInvalidFinalResponse(t *testing.T) {
	read := &correctingReadTool{}
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(" ", nil, "stop"),
		llm.NewLlmResponse("recovered final answer", nil, "stop"),
	}}
	loop, err := NewRegistryToolLoop(testLiveAssembler(t), client, []agenttool.Tool{read}, []string{read.Name()}, turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 1})
	if err != nil {
		t.Fatal(err)
	}
	inv := runtime.NewInvocation("final-reflection", runtime.NewContext("room", "caller", "", "", false, nil), "answer me", runtime.MENTION, "", false)
	completion, err := loop.CompleteWithEvidence(context.Background(), inv, nil, "")
	if err != nil || completion.Response.Content() != "recovered final answer" || len(client.requests) != 2 {
		t.Fatalf("completion=%#v requests=%d err=%v", completion, len(client.requests), err)
	}
	if len(client.requests[1].Tools()) != 0 {
		t.Fatalf("reflection exposed tools: %#v", client.requests[1].Tools())
	}
}
