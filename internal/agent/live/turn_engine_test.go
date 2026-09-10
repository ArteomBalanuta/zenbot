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

type scriptedCompletionGate struct {
	assessments []turn.CompletionAssessment
	inputs      []turn.CompletionCandidate
}

type acceptingCompletionGate struct{}

func (acceptingCompletionGate) Evaluate(context.Context, turn.CompletionCandidate) (turn.CompletionAssessment, error) {
	return turn.CompletionAssessment{Decision: turn.CompletionFinal, Feedback: "The candidate satisfies the request."}, nil
}

func (g *scriptedCompletionGate) Evaluate(_ context.Context, candidate turn.CompletionCandidate) (turn.CompletionAssessment, error) {
	g.inputs = append(g.inputs, candidate)
	assessment := g.assessments[0]
	g.assessments = g.assessments[1:]
	return assessment, nil
}

func TestTurnEngineSemanticGateContinuesPromiseUntilToolResultSatisfiesRequest(t *testing.T) {
	action := &engineActionTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{action}, []string{action.Name()})
	agent, _ := api.NewContext("room", "caller", "", "", false, nil)
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("action-1", action.Name(), map[string]any{})}, "tool_calls"),
		llm.NewLlmResponse("The action completed successfully.", nil, "stop"),
	}}
	gate := &scriptedCompletionGate{assessments: []turn.CompletionAssessment{
		{Decision: turn.CompletionContinue, Feedback: "Execute the available action before answering."},
		{Decision: turn.CompletionFinal, Feedback: "The requested action is complete and reported."},
	}}
	limits := turn.ExecutionLimits{MaxSteps: 4, MaxToolCalls: 1, MaxCallsPerTool: 1}
	state := turn.NewState(limits)
	state.AdvanceStep()
	engine := TurnEngine{
		Client:    client,
		Registry:  registry,
		Agent:     agent,
		Allowed:   []string{action.Name()},
		Limits:    limits,
		Interrupt: fixedInterruptHook{decision: turn.InterruptDecision{Outcome: turn.InterruptAllow}},
		Gate:      gate,
		Prompt:    "perform the available action",
	}
	initial := llm.NewLlmResponse("I will execute that action now.", nil, "stop")
	messages := []llm.LlmMessage{llm.NewLlmMessage("user", "perform the available action", nil, "")}
	providerTools := []any{map[string]any{"type": "function", "function": map[string]any{"name": action.Name(), "description": "Execute the test action."}}}

	response, _, batch, err := engine.Complete(context.Background(), messages, providerTools, initial, state)
	if err != nil {
		t.Fatal(err)
	}
	if response.Content() != "The action completed successfully." || action.calls.Load() != 1 || len(batch) != 1 {
		t.Fatalf("response=%q executions=%d batch=%d", response.Content(), action.calls.Load(), len(batch))
	}
	if len(gate.inputs) != 2 || !gate.inputs[0].CanContinue || !gate.inputs[1].CanContinue || len(gate.inputs[0].Results) != 0 || len(gate.inputs[1].Results) != 1 {
		t.Fatalf("gate inputs=%#v", gate.inputs)
	}
	if gate.inputs[1].Results[0].ToolName != action.Name() || gate.inputs[1].Results[0].IsError {
		t.Fatalf("gate did not receive successful observation: %#v", gate.inputs[1].Results)
	}
	if len(client.requests) != 2 || len(client.requests[0].Tools()) != 1 || len(client.requests[1].Tools()) != 1 {
		t.Fatalf("tools were not retained across continuation: requests=%d", len(client.requests))
	}
	correctionMessages := client.requests[0].Messages()
	if !strings.Contains(correctionMessages[len(correctionMessages)-1].Content(), "Execute the available action") {
		t.Fatalf("semantic feedback missing from retry: %#v", correctionMessages)
	}
	observationMessages := client.requests[1].Messages()
	if !strings.Contains(observationMessages[len(observationMessages)-1].Content(), `"executed":true`) {
		t.Fatalf("tool observation missing from follow-up: %#v", observationMessages)
	}
}

func TestTurnEngineDoesNotPublishUnsatisfiedCandidateAtStepLimit(t *testing.T) {
	action := &engineActionTool{}
	registry := agenttool.NewRegistry([]agenttool.Tool{action}, []string{action.Name()})
	agent, _ := api.NewContext("room", "caller", "", "", false, nil)
	gate := &scriptedCompletionGate{assessments: []turn.CompletionAssessment{{Decision: turn.CompletionContinue, Feedback: "The action has not run."}}}
	limits := turn.ExecutionLimits{MaxSteps: 1, MaxToolCalls: 1, MaxCallsPerTool: 1}
	state := turn.NewState(limits)
	state.AdvanceStep()
	engine := TurnEngine{Client: &scriptedToolClient{}, Registry: registry, Agent: agent, Allowed: []string{action.Name()}, Limits: limits, Gate: gate, Prompt: "perform the action"}

	_, _, _, err := engine.Complete(context.Background(), []llm.LlmMessage{llm.NewLlmMessage("user", "perform the action", nil, "")}, []any{"manifest"}, llm.NewLlmResponse("I will do it.", nil, "stop"), state)
	if err == nil || !strings.Contains(err.Error(), "semantic completion remains unsatisfied at step limit") {
		t.Fatalf("error=%v", err)
	}
	if len(gate.inputs) != 1 || gate.inputs[0].CanContinue {
		t.Fatalf("gate continuation availability=%#v", gate.inputs)
	}
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
		Gate:      &scriptedCompletionGate{assessments: []turn.CompletionAssessment{{Decision: turn.CompletionFinal, Feedback: "The denial is accurately reported."}}},
		Prompt:    "perform the action",
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
		Gate:      &scriptedCompletionGate{},
		Prompt:    "perform the action",
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
