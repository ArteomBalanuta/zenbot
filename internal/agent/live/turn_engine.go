package live

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/observability"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
	"zenbot/internal/agent/turn"
)

// TurnEngine executes the model's chosen calls and returns their observations.
// Intent, planning, recovery and completion belong to the model.
type TurnEngine struct {
	Client        llm.LlmClient
	Registry      *tool.Registry
	Agent         api.Context
	Projector     *assemble.Assembler
	NewestRequest llm.LlmMessage
	Observations  *assemble.ObservationStore
	Allowed       []string
	Limits        turn.ExecutionLimits
	Prompt        string
	NoReplyMarker string
	AllowSilence  bool
	Control       *CompletionControl
}

type CompletionControl struct{ SuppressReply bool }

func (e TurnEngine) Complete(ctx context.Context, messages []llm.LlmMessage, providerTools []any, initial llm.LlmResponse, state *turn.State) (llm.LlmResponse, []llm.LlmMessage, []toolBatchResult, error) {
	if e.Client == nil || e.Registry == nil || e.Projector == nil || e.Observations == nil || state == nil {
		return llm.LlmResponse{}, messages, nil, fmt.Errorf("agent turn engine is incomplete")
	}
	executor := &execution.Executor{
		Registry:       e.Registry,
		Ledger:         execution.NewLedger(toolBudgets(e.Allowed, e.Limits), normalizedFailureLimit(e.Limits)),
		Exposed:        providerToolNames(providerTools),
		DefaultTimeout: e.Limits.ToolTimeout,
	}
	response := initial
	all := make([]toolBatchResult, 0)
	terminal, repaired := false, false
	for cycle := 1; ; cycle++ {
		if err := ctx.Err(); err != nil {
			return response, messages, all, err
		}
		calls := response.ToolCalls()
		observability.Info(ctx, "agent.loop.cycle", "cycle", cycle, "tool_call_count", len(calls), "finish_reason", response.FinishReason())
		if len(calls) == 0 && response.FinishReason() != "length" {
			if err := e.validateFinal(response, all); err == nil {
				if e.Control != nil {
					e.Control.SuppressReply = e.isSilence(response.Content()) && canSuppressCompletedReply(e.Registry, e.Agent, all)
				}
				return response, messages, all, nil
			}
		}
		feedback := ""
		if len(calls) == 0 {
			if repaired {
				return response, messages, all, fmt.Errorf("invalid final response after structural correction")
			}
			repaired = true
			feedback = "The last response was empty, truncated, contained tool protocol markup, or requested silence without a delivered answer. Return a complete answer, or make the necessary native tool calls while tools remain available."
			messages = append(messages, llm.NewLlmMessage("assistant", response.Content(), nil, ""))
		} else {
			batchCalls := make([]execution.Call, len(calls))
			for i, call := range calls {
				batchCalls[i] = execution.FromLLM(call)
			}
			protocolErr := execution.ValidateBatchIdentity(batchCalls)
			if protocolErr == nil {
				for _, call := range batchCalls {
					if _, exists := e.Observations.Full(call.ID); exists {
						protocolErr = fmt.Errorf("tool call ID %q was already used; use a fresh ID", call.ID)
						break
					}
				}
			}
			if response.FinishReason() == "length" || protocolErr != nil {
				if repaired {
					return response, messages, all, fmt.Errorf("invalid tool protocol after structural correction")
				}
				repaired = true
				feedback = "No calls from the last response were executed. Return complete native tool calls with nonblank names and fresh unique IDs."
				if protocolErr != nil {
					feedback += " " + protocolErr.Error()
				}
				// Invalid protocol cannot form an assistant/tool pair; keep it as data.
				raw, _ := json.Marshal(callsForFeedback(calls))
				feedback += " Rejected calls: " + string(raw)
			} else {
				batch := e.executeBatch(ctx, executor, state, batchCalls, terminal)
				all = append(all, batch...)
				var err error
				messages, err = appendRegistryProtocol(messages, response, batch, e.Observations, e.Registry, e.Agent)
				if err != nil {
					return response, messages, all, err
				}
				if terminal {
					if repaired {
						return response, messages, all, fmt.Errorf("provider called tools after terminal synthesis")
					}
					repaired = true
					feedback = "Those calls were not executed because this turn has no remaining tool budget. Give the best supported final answer, including any unfinished work."
				}
			}
		}
		if feedback != "" {
			messages = append(messages, llm.NewLlmMessage("user", "TOOL_LOOP_FEEDBACK="+feedback, nil, ""))
		}
		tools := e.availableProviderTools(providerTools, executor.Ledger)
		stage := fmt.Sprintf("llm.tool_follow_up.%d", cycle)
		if terminal || state.RemainingToolCalls() == 0 || len(tools) == 0 || !state.AdvanceStep() {
			terminal = true
			tools = nil
			stage = "llm.terminal_synthesis"
		}
		// Status is current-round metadata, not accumulated advice or a plan.
		status, _ := json.Marshal(map[string]any{
			"remainingModelRounds":   state.RemainingSteps(),
			"remainingToolCalls":     state.RemainingToolCalls(),
			"toolsAvailable":         len(tools) > 0,
			"terminal":               terminal,
			"blockedByUnknownAction": executor.Ledger.UnknownActionCallID(),
		})
		statusMessage := llm.NewLlmMessage("user", "TOOL_LOOP_STATUS="+string(status), nil, "")
		request, err := e.projectRequest(messages, tools, statusMessage)
		if err != nil {
			return response, messages, all, err
		}
		if err := ctx.Err(); err != nil {
			return response, messages, all, err
		}
		response, err = e.Client.Complete(observability.WithStage(ctx, stage), request)
		if err != nil {
			return response, messages, all, err
		}
	}
}

func callsForFeedback(calls []llm.LlmToolCall) []map[string]string {
	out := make([]map[string]string, 0, len(calls))
	for _, call := range calls {
		raw := []rune(call.RawArguments())
		if len(raw) > 2000 {
			raw = raw[:2000]
		}
		out = append(out, map[string]string{"id": call.ID(), "name": call.Name(), "arguments": string(raw)})
	}
	return out
}

func (e TurnEngine) executeBatch(ctx context.Context, executor *execution.Executor, state *turn.State, calls []execution.Call, terminal bool) []toolBatchResult {
	results := make([]contract.Result, len(calls))
	budgetOK := !terminal && state.ReserveToolCalls(len(calls))
	_ = state.MarkToolAttempted(len(calls))
	if budgetOK {
		results = execution.ExecuteAll(ctx, executor, e.Agent, calls)
	} else {
		for i, call := range calls {
			results[i] = contract.ActionErrorResult(call.ID, call.Name, "TOOL_CALL_LIMIT_REACHED", "This batch was not executed. Request fewer calls if budget remains; otherwise summarize the available results and unfinished work.", contract.EffectNotStarted)
		}
	}
	batch := make([]toolBatchResult, len(calls))
	for i, call := range calls {
		if results[i].IsError {
			_ = state.RecordToolFailure()
		} else {
			_ = state.RecordToolSuccess()
		}
		batch[i] = toolBatchResult{Call: call, Result: results[i]}
	}
	return batch
}

func (e TurnEngine) isSilence(content string) bool {
	marker := e.NoReplyMarker
	if marker == "" {
		marker = "NO_REPLY"
	}
	return strings.TrimSpace(content) == marker
}

func (e TurnEngine) validateFinal(response llm.LlmResponse, batch []toolBatchResult) error {
	if err := (turn.FinalResponseValidator{}).Validate(turn.FinalResponseInput{Response: response}); err != nil {
		return err
	}
	if e.isSilence(response.Content()) && !e.AllowSilence && !canSuppressCompletedReply(e.Registry, e.Agent, batch) {
		return fmt.Errorf("silence requires a verified delivered answer")
	}
	return nil
}

func canSuppressCompletedReply(registry *tool.Registry, agent api.Context, batch []toolBatchResult) bool {
	hasDelivery := false
	for _, item := range batch {
		if item.Result.ErrorCode == "ACTION_OUTCOME_UNKNOWN" {
			return false
		}
		registered, found := registry.Lookup(item.Call.Name)
		if !found {
			continue
		}
		descriptor, err := registered.Descriptor(agent)
		if err != nil {
			continue
		}
		if descriptor.Effect() == contract.Action && descriptor.ResultMode() != contract.RoomDelivery && item.Result.EffectsCommitted {
			return false
		}
		hasDelivery = hasDelivery || item.Result.VerifiedRoomDelivery()
	}
	return hasDelivery
}

func (e TurnEngine) projectRequest(messages []llm.LlmMessage, tools []any, requiredRuntime ...llm.LlmMessage) (llm.LlmRequest, error) {
	projection, err := e.Projector.ProjectTurn(messages, tools, e.Observations, e.NewestRequest, requiredRuntime...)
	if err != nil {
		return llm.LlmRequest{}, fmt.Errorf("project agent turn: %w", err)
	}
	return llm.NewLlmRequest(projection.Messages, tools, false, nil, projection), nil
}

func providerToolNames(definitions []any) map[string]bool {
	names := make(map[string]bool, len(definitions))
	for _, raw := range definitions {
		definition, _ := raw.(map[string]any)
		function, _ := definition["function"].(map[string]any)
		name, _ := function["name"].(string)
		if name != "" {
			names[name] = true
		}
	}
	return names
}

func (e TurnEngine) availableProviderTools(providerTools []any, ledger *execution.Ledger) []any {
	available := make([]any, 0, len(providerTools))
	for _, raw := range providerTools {
		definition, _ := raw.(map[string]any)
		function, _ := definition["function"].(map[string]any)
		name, _ := function["name"].(string)
		if name != "" && (ledger == nil || ledger.Available(name)) {
			if ledger.UnknownActionCallID() != "" {
				registered, found := e.Registry.Lookup(name)
				if !found {
					continue
				}
				descriptor, err := registered.Descriptor(e.Agent)
				if err != nil || descriptor.Effect() == contract.Action {
					continue
				}
			}
			available = append(available, raw)
		}
	}
	return available
}

func toolBudgets(allowed []string, limits turn.ExecutionLimits) map[string]int {
	perTool := limits.MaxCallsPerTool
	if perTool <= 0 {
		perTool = limits.MaxToolCalls
	}
	budgets := make(map[string]int, len(allowed))
	for _, name := range allowed {
		budgets[name] = perTool
	}
	budgets[readToolResultName] = limits.MaxToolCalls
	return budgets
}

func normalizedFailureLimit(limits turn.ExecutionLimits) int {
	if limits.MaxToolFailures > 0 {
		return limits.MaxToolFailures
	}
	return 2
}
