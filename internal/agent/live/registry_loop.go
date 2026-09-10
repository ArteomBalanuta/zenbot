package live

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/observability"
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
	for _, call := range calls {
		if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" {
			return nil, nil, fmt.Errorf("invalid agent tool call")
		}
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
		if item.Call.ID != calls[i].ID() || item.Result.ToolName != item.Call.Name {
			return nil, fmt.Errorf("agent tool protocol identity mismatch")
		}
		messages = append(messages, llm.NewLlmMessage("tool", string(item.Result.Envelope()), nil, item.Call.ID))
	}
	return messages, nil
}

func completeRegistryLoop(ctx context.Context, client llm.LlmClient, registry *tool.Registry, agent api.Context, messages []llm.LlmMessage, providerTools []any, initial llm.LlmResponse, allowed []string, limits turn.ExecutionLimits, state *turn.State) (llm.LlmResponse, []llm.LlmMessage, []toolBatchResult, error) {
	if client == nil || registry == nil || state == nil {
		return llm.LlmResponse{}, nil, nil, fmt.Errorf("agent registry loop is incomplete")
	}
	budget := make(map[string]int, len(allowed))
	perToolLimit := limits.MaxCallsPerTool
	if perToolLimit <= 0 {
		perToolLimit = limits.MaxToolCalls
	}
	for _, name := range allowed {
		budget[name] = perToolLimit
	}
	maxFailures := limits.MaxToolFailures
	if maxFailures <= 0 {
		maxFailures = 2
	}
	executor := &execution.Executor{Registry: registry, Ledger: execution.NewLedger(budget, maxFailures), DefaultTimeout: limits.ToolTimeout}
	response := initial
	all := []toolBatchResult{}
	cycle := 0
	for {
		cycle++
		if response.FinishReason() == "length" {
			return llm.LlmResponse{}, nil, nil, fmt.Errorf("agent response was truncated")
		}
		calls := response.ToolCalls()
		observability.Info(ctx, "agent.loop.cycle", "cycle", cycle, "tool_call_count", len(calls), "finish_reason", response.FinishReason())
		if len(calls) == 0 {
			return response, messages, all, nil
		}
		batchCalls := make([]execution.Call, 0, len(calls))
		for _, call := range calls {
			batchCalls = append(batchCalls, execution.FromLLM(call))
		}
		observability.Info(ctx, "agent.tool.batch_started", "cycle", cycle, "tool_call_count", len(batchCalls))
		_, batch, err := executeRegistryBatch(ctx, executor, agent, limits, state, batchCalls)
		if err != nil {
			return llm.LlmResponse{}, nil, nil, err
		}
		failures := 0
		for _, item := range batch {
			if item.Result.IsError {
				failures++
			}
		}
		observability.Info(ctx, "agent.tool.batch_completed", "cycle", cycle, "tool_call_count", len(batch), "failure_count", failures)
		all = append(all, batch...)
		messages, err = appendRegistryProtocol(messages, response, batch)
		if err != nil {
			return llm.LlmResponse{}, nil, nil, err
		}
		if !state.AdvanceStep() {
			return llm.LlmResponse{}, nil, nil, fmt.Errorf("agent execution step limit reached")
		}
		response, err = client.Complete(observability.WithStage(ctx, fmt.Sprintf("llm.tool_follow_up.%d", cycle)), llm.NewLlmRequest(messages, providerTools, false, nil, nil))
		if err != nil {
			return llm.LlmResponse{}, nil, nil, err
		}
		if err := ctx.Err(); err != nil {
			return llm.LlmResponse{}, nil, nil, err
		}
	}
}

type toolBatchResult struct {
	Call   execution.Call
	Result contract.Result
}
