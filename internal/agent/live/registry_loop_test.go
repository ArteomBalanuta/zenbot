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
}
