package live

import (
	"context"
	"fmt"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
	"zenbot/internal/agent/turn"
)

// executeRegistryBatch is the shared serial execution seam for Saturn's
// generalized tool loop. It preserves descriptor prerequisites through the
// ledger and produces one result for every provider call in source order.
func executeRegistryBatch(ctx context.Context, executor *execution.Executor, agent api.Context, limits turn.ExecutionLimits, state *turn.State, calls []execution.Call) ([]execution.Call, []toolBatchResult, error) {
	if executor == nil || executor.Registry == nil || state == nil || len(calls) == 0 {
		return nil, nil, fmt.Errorf("agent registry batch is incomplete")
	}
	if len(calls) > limits.MaxToolCalls || !state.ReserveToolCalls(len(calls)) {
		return nil, nil, fmt.Errorf("agent tool call limit")
	}
	if err := execution.ValidateBatchIdentity(calls); err != nil {
		return nil, nil, fmt.Errorf("invalid agent tool call: %w", err)
	}
	if err := state.MarkToolAttempted(len(calls)); err != nil {
		return nil, nil, err
	}
	results := execution.ExecuteAll(ctx, executor, agent, calls)
	out := make([]toolBatchResult, 0, len(calls))
	for index, call := range calls {
		result := results[index]
		if result.IsError {
			_ = state.RecordToolFailure()
		} else {
			_ = state.RecordToolSuccess()
		}
		out = append(out, toolBatchResult{Call: call, Result: result})
	}
	return calls, out, nil
}

func appendRegistryProtocol(messages []llm.LlmMessage, response llm.LlmResponse, batch []toolBatchResult, observations *assemble.ObservationStore, registry *tool.Registry, agent api.Context) ([]llm.LlmMessage, error) {
	calls := response.ToolCalls()
	if len(calls) != len(batch) || observations == nil {
		return nil, fmt.Errorf("agent tool protocol cardinality mismatch")
	}
	var content any
	if response.ContentNullable() != nil {
		content = response.Content()
	}
	messages = append(messages, llm.NewLlmMessage("assistant", content, calls, ""))
	for i, item := range batch {
		if item.Call.ID != calls[i].ID() || item.Result.CallID != item.Call.ID || item.Result.ToolName != item.Call.Name {
			return nil, fmt.Errorf("agent tool protocol identity mismatch")
		}
		maxBytes := contract.DefaultMaxModelResultBytes
		if registered, found := registry.Lookup(item.Call.Name); found {
			if descriptor, err := registered.Descriptor(agent); err == nil {
				maxBytes = descriptor.MaxModelResultBytes()
			}
		}
		view := observations.Store(item.Result, maxBytes)
		encoded := view.JSON()
		if len(encoded) > maxBytes {
			return nil, fmt.Errorf("agent tool observation metadata exceeds model result limit")
		}
		messages = append(messages, llm.NewLlmMessage("tool", string(encoded), nil, item.Call.ID))
	}
	return messages, nil
}

func completeRegistryLoop(ctx context.Context, client llm.LlmClient, registry *tool.Registry, agent api.Context, projector *assemble.Assembler, newestRequest llm.LlmMessage, observations *assemble.ObservationStore, messages []llm.LlmMessage, providerTools []any, initial llm.LlmResponse, allowed []string, limits turn.ExecutionLimits, state *turn.State, interrupt turn.InterruptHook, gate turn.CompletionGate, prompt string) (llm.LlmResponse, []llm.LlmMessage, []toolBatchResult, bool, error) {
	control := &CompletionControl{}
	response, completedMessages, batch, err := (TurnEngine{Client: client, Registry: registry, Agent: agent, Projector: projector, NewestRequest: newestRequest, Observations: observations, Allowed: allowed, Limits: limits, Interrupt: interrupt, Gate: gate, Prompt: prompt, Control: control}).Complete(ctx, messages, providerTools, initial, state)
	return response, completedMessages, batch, control.SuppressReply, err
}

type toolBatchResult struct {
	Call   execution.Call
	Result contract.Result
}
