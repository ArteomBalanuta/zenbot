package live

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
	"zenbot/internal/agent/turn"
)

const respondToUserTool = "respond_to_user"

func respondToUserDefinition() any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        respondToUserTool,
			"description": "Return the final answer only when no Saturn tool is needed, or after all required tool work is complete. Do NOT use this to describe, promise, plan, simulate, or claim a tool action. If the newest request asks to execute a command or retrieve live data and a matching tool is exposed, call that tool instead.",
			"parameters": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"response": map[string]any{"type": "string", "minLength": 1},
				},
				"required": []string{"response"},
			},
		},
	}
}

func providerHasTool(tools []any, name string) bool {
	for _, raw := range tools {
		definition, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		function, ok := definition["function"].(map[string]any)
		if ok && function["name"] == name {
			return true
		}
	}
	return false
}

func structuredFinalResponse(response llm.LlmResponse) (llm.LlmResponse, bool, error) {
	calls := response.ToolCalls()
	found := -1
	for index, call := range calls {
		if call.Name() == respondToUserTool {
			if found >= 0 || len(calls) != 1 {
				return llm.LlmResponse{}, false, fmt.Errorf("final answer control call cannot be combined with tool calls")
			}
			found = index
		}
	}
	if found < 0 {
		return llm.LlmResponse{}, false, nil
	}
	call := calls[found]
	if strings.TrimSpace(call.ID()) == "" {
		return llm.LlmResponse{}, false, fmt.Errorf("final answer control call has no id")
	}
	arguments := call.Arguments()
	if len(arguments) != 1 {
		return llm.LlmResponse{}, false, fmt.Errorf("invalid final answer control arguments")
	}
	content, ok := arguments["response"].(string)
	if !ok || strings.TrimSpace(content) == "" {
		return llm.LlmResponse{}, false, fmt.Errorf("invalid final answer control response")
	}
	if raw := strings.TrimSpace(call.RawArguments()); raw == "" || !json.Valid([]byte(raw)) {
		return llm.LlmResponse{}, false, fmt.Errorf("invalid final answer control payload")
	}
	return llm.NewLlmResponse(strings.TrimSpace(content), nil, "stop"), true, nil
}

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

func completeRegistryLoop(ctx context.Context, client llm.LlmClient, registry *tool.Registry, agent api.Context, messages []llm.LlmMessage, providerTools []any, initial llm.LlmResponse, allowed []string, limits turn.ExecutionLimits, state *turn.State, interrupt turn.InterruptHook) (llm.LlmResponse, []llm.LlmMessage, []toolBatchResult, error) {
	return (TurnEngine{Client: client, Registry: registry, Agent: agent, Allowed: allowed, Limits: limits, Interrupt: interrupt}).Complete(ctx, messages, providerTools, initial, state)
}

type toolBatchResult struct {
	Call   execution.Call
	Result contract.Result
}
