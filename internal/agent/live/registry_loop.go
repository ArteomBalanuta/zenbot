package live

import (
	"context"
	"fmt"

	"zenbot/internal/agent/api"
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

func appendRegistryProtocol(messages []llm.LlmMessage, response llm.LlmResponse, batch []toolBatchResult) ([]llm.LlmMessage, error) {
	calls := response.ToolCalls()
	if len(calls) != len(batch) {
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
		messages = append(messages, llm.NewLlmMessage("tool", string(item.Result.Envelope()), nil, item.Call.ID))
	}
	return messages, nil
}

func completeRegistryLoop(ctx context.Context, client llm.LlmClient, registry *tool.Registry, agent api.Context, messages []llm.LlmMessage, providerTools []any, initial llm.LlmResponse, allowed []string, limits turn.ExecutionLimits, state *turn.State, interrupt turn.InterruptHook, gate turn.CompletionGate, prompt string) (llm.LlmResponse, []llm.LlmMessage, []toolBatchResult, error) {
	return (TurnEngine{Client: client, Registry: registry, Agent: agent, Allowed: allowed, Limits: limits, Interrupt: interrupt, Gate: gate, Prompt: prompt}).Complete(ctx, messages, providerTools, initial, state)
}

type toolBatchResult struct {
	Call   execution.Call
	Result contract.Result
}
