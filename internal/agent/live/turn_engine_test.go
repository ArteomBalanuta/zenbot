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
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
	"zenbot/internal/agent/turn"
)

type engineActionTool struct {
	calls        atomic.Int32
	capabilities []string
}

func (t *engineActionTool) Name() string { return "engine_action" }
func (t *engineActionTool) Descriptor(api.Context) (contract.Descriptor, error) {
	return contract.NewDescriptor(t.Name(), "Engine action", "Execute a state-changing test action.", "test", contract.AccessUser, contract.Action, contract.ModelData, json.RawMessage(`{"type":"object","additionalProperties":false}`), t.capabilities, nil, false, time.Second, json.RawMessage(`{"type":"object"}`), nil, []string{"room"}, []string{"Do not use for reads."})
}
func (t *engineActionTool) Execute(context.Context, api.Context, json.RawMessage) (contract.Result, error) {
	t.calls.Add(1)
	return contract.SuccessResult("", t.Name(), map[string]any{"executed": true}), nil
}

type fixedInterruptHook struct{ decision turn.InterruptDecision }

func (h fixedInterruptHook) Review(context.Context, turn.ActionRequest) (turn.InterruptDecision, error) {
	return h.decision, nil
}

func TestTurnEngineFeedsDeniedActionBackAsObservation(t *testing.T) {
	action := &engineActionTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{action}, []string{action.Name()})
	agent, _ := api.NewContext("room", "caller", "", "", false, nil)
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("The action was not executed.", nil, "stop")}}
	state := turn.NewState(turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 1})
	state.AdvanceStep()
	engine := TurnEngine{
		Client:    client,
		Registry:  registry,
		Agent:     agent,
		Allowed:   []string{action.Name()},
		Limits:    turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 1},
		Interrupt: fixedInterruptHook{decision: turn.InterruptDecision{Outcome: turn.InterruptDeny, Reason: "operator denied"}},
	}
	initial := llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("action-1", action.Name(), map[string]any{})}, "tool_calls")
	response, _, batch, err := engine.Complete(context.Background(), nil, []any{"manifest"}, initial, state)
	if err != nil {
		t.Fatal(err)
	}
	if action.calls.Load() != 0 || response.Content() != "The action was not executed." || len(batch) != 1 || batch[0].Result.ErrorCode != "ACTION_DENIED" {
		t.Fatalf("response=%#v batch=%#v executions=%d", response, batch, action.calls.Load())
	}
	requestMessages := client.requests[0].Messages()
	if !strings.Contains(requestMessages[len(requestMessages)-1].Content(), "ACTION_DENIED") {
		t.Fatalf("denial observation missing: %#v", requestMessages)
	}
}

func TestTurnEnginePausesBeforeExecutingAction(t *testing.T) {
	action := &engineActionTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{action}, []string{action.Name()})
	agent, _ := api.NewContext("room", "caller", "", "", false, nil)
	state := turn.NewState(turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 1})
	state.AdvanceStep()
	engine := TurnEngine{
		Client:    &scriptedToolClient{},
		Registry:  registry,
		Agent:     agent,
		Allowed:   []string{action.Name()},
		Limits:    turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 1},
		Interrupt: fixedInterruptHook{decision: turn.InterruptDecision{Outcome: turn.InterruptPause, Reason: "approval required", ResumeToken: "resume-1"}},
	}
	initial := llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("action-1", action.Name(), map[string]any{})}, "tool_calls")
	_, _, _, err := engine.Complete(context.Background(), nil, nil, initial, state)
	var paused *turn.PausedError
	if !errors.As(err, &paused) || paused.Pending.ResumeToken != "resume-1" {
		t.Fatalf("error=%#v, want paused checkpoint", err)
	}
	if action.calls.Load() != 0 {
		t.Fatalf("action executed %d times before approval", action.calls.Load())
	}
}

func TestTurnEngineResumeActionRevalidatesCurrentAuthorization(t *testing.T) {
	action := &engineActionTool{capabilities: []string{string(api.ModerationCommands)}}
	registry := agenttool.NewRegistry([]agenttool.Tool{action}, []string{action.Name()})
	authorized, _ := api.NewContextWithCapabilities("room", "caller", "", "", false, []string{}, []api.Capability{api.ModerationCommands})
	current, _ := api.NewContext("room", "caller", "", "", false, []string{})
	call := execution.Call{ID: "action-1", Name: action.Name(), Arguments: json.RawMessage(`{}`)}
	descriptor, _ := action.Descriptor(authorized)
	pending := turn.PendingAction{
		Request:     turn.ActionRequest{Call: call, Descriptor: descriptor, Context: authorized},
		ResumeToken: "resume-1",
		Reason:      "approval required",
	}
	engine := TurnEngine{Registry: registry, Allowed: []string{action.Name()}, Limits: turn.ExecutionLimits{MaxToolCalls: 1}}

	result, err := engine.ResumeAction(context.Background(), pending, "resume-1", current)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.ErrorCode != "TOOL_NOT_AUTHORIZED" || action.calls.Load() != 0 {
		t.Fatalf("result=%#v executions=%d", result, action.calls.Load())
	}
}

func TestTurnEngineResumeActionRejectsWrongToken(t *testing.T) {
	action := &engineActionTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{action}, []string{action.Name()})
	agent, _ := api.NewContext("room", "caller", "", "", false, []string{})
	call := execution.Call{ID: "action-1", Name: action.Name(), Arguments: json.RawMessage(`{}`)}
	descriptor, _ := action.Descriptor(agent)
	pending := turn.PendingAction{Request: turn.ActionRequest{Call: call, Descriptor: descriptor, Context: agent}, ResumeToken: "resume-1", Reason: "approval required"}
	engine := TurnEngine{Registry: registry, Allowed: []string{action.Name()}, Limits: turn.ExecutionLimits{MaxToolCalls: 1}}

	if _, err := engine.ResumeAction(context.Background(), pending, "wrong", agent); err == nil {
		t.Fatal("resume with a mismatched token succeeded")
	}
	if action.calls.Load() != 0 {
		t.Fatalf("action executed %d times with a mismatched token", action.calls.Load())
	}
}
