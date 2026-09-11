package live

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/llm"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/turn"
)

type observedFailureAction struct {
	engineActionTool
	observed bool
}

func (t *observedFailureAction) Execute(context.Context, api.Context, json.RawMessage) (contract.Result, error) {
	t.calls.Add(1)
	r := contract.ActionErrorResult("", t.Name(), "ACTION_OUTCOME_UNKNOWN", strings.Repeat("safe acknowledgment failure ", 1000), contract.EffectUnknown)
	if t.observed {
		r.ObservedData = json.RawMessage(`{"room":"lounge","count":2,"users":["alice","bob"]}`)
	}
	return r, nil
}

func TestThreeRoundFinalContextRetainsErrorFactsAndUnknownReceipt(t *testing.T) {
	for _, observed := range []bool{true, false} {
		t.Run(map[bool]string{true: "observed", false: "no source data"}[observed], func(t *testing.T) {
			action := &observedFailureAction{observed: observed}
			read := &correctingReadTool{}
			registry := agenttool.NewRegistry([]agenttool.Tool{action, read}, []string{action.Name(), read.Name()})
			client := &scriptedToolClient{responses: []llm.LlmResponse{
				llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("two", read.Name(), map[string]any{"value": "second"})}, "tool_calls"),
				llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("three", read.Name(), map[string]any{"value": "third"})}, "tool_calls"),
				llm.NewLlmResponse("Final synthesis.", nil, "stop"),
			}}
			limits := turn.ExecutionLimits{MaxSteps: 4, MaxToolCalls: 3, MaxCallsPerTool: 2}
			engine := configuredTurnEngine(t, TurnEngine{Client: client, Registry: registry, Agent: mustAgentContext(t), Allowed: []string{action.Name(), read.Name()}, Limits: limits, Prompt: "Inspect remote source and two more readings, then explain."})
			projector, err := assemble.New(assemble.Config{MaxContextTokens: 2400, ContextReserveTokens: 100}, tinyBudgetCatalog{})
			if err != nil {
				t.Fatal(err)
			}
			engine.Projector = projector
			definitions, _ := providerToolDefinitions(registry, engine.Agent)
			state := turn.NewState(limits)
			state.AdvanceStep()
			initial := llm.NewLlmResponse(nil, []llm.LlmToolCall{llm.NewLlmToolCall("remote", action.Name(), map[string]any{})}, "tool_calls")
			_, _, batch, err := engine.Complete(context.Background(), engineMessages(engine.Prompt), definitions, initial, state)
			if err != nil || len(batch) != 3 || len(client.requests) != 3 || action.calls.Load() != 1 {
				t.Fatalf("batch=%d requests=%d err=%v", len(batch), len(client.requests), err)
			}
			for i, request := range client.requests {
				assertRequestWithinBudget(t, i, request, 2400, 100)
				assertAtomicToolProtocol(t, i, request.Messages())
				if messagesContain(request.Messages(), `"room":"lounge"`) != observed || messagesContain(request.Messages(), `"count":2`) != observed {
					t.Fatalf("round %d lost/invented observed facts", i)
				}
				if !messagesContain(request.Messages(), "ACTION_OUTCOME_UNKNOWN") || !messagesContain(request.Messages(), `"effectState":"UNKNOWN"`) || !messagesContain(request.Messages(), `"deliveryCount":0`) {
					t.Fatalf("round %d lost unknown receipt", i)
				}
			}
		})
	}
}
