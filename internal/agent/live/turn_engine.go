package live

import (
	"context"
	"crypto/subtle"
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

// TurnEngine owns the request-local model/tool feedback loop. It keeps the
// model transcript, execution state, recovery bounds, and interruption policy
// isolated from every other request.
type TurnEngine struct {
	Client        llm.LlmClient
	Registry      *tool.Registry
	Agent         api.Context
	Projector     *assemble.Assembler
	NewestRequest llm.LlmMessage
	Observations  *assemble.ObservationStore
	Allowed       []string
	Limits        turn.ExecutionLimits
	Interrupt     turn.InterruptHook
	Final         turn.FinalResponseValidator
	Recovery      turn.RecoveryPolicy
	Gate          turn.CompletionGate
	Prompt        string
	Control       *CompletionControl
}

// CompletionControl carries the semantic finalizer's delivery disposition out
// of the engine without deriving intent from keywords or tool-batch size.
type CompletionControl struct {
	SuppressReply bool
}

func (e TurnEngine) Complete(ctx context.Context, messages []llm.LlmMessage, providerTools []any, initial llm.LlmResponse, state *turn.State) (llm.LlmResponse, []llm.LlmMessage, []toolBatchResult, error) {
	if e.Client == nil || e.Registry == nil || e.Gate == nil || e.Projector == nil || e.Observations == nil || state == nil || strings.TrimSpace(e.Prompt) == "" {
		return llm.LlmResponse{}, nil, nil, fmt.Errorf("agent turn engine is incomplete")
	}
	hook := e.Interrupt
	if hook == nil {
		hook = turn.AllowAllInterruptHook{}
	}
	phase := turn.NewPhaseMachine()
	if err := phase.Transition(turn.PhaseModel); err != nil {
		return llm.LlmResponse{}, nil, nil, err
	}
	executor := &execution.Executor{
		Registry:       e.Registry,
		Ledger:         execution.NewLedger(toolBudgets(e.Allowed, e.Limits), normalizedFailureLimit(e.Limits)),
		DefaultTimeout: e.Limits.ToolTimeout,
	}
	response := initial
	all := make([]toolBatchResult, 0)
	cycle := 0
	for {
		cycle++
		calls := response.ToolCalls()
		observability.Info(ctx, "agent.loop.cycle", "cycle", cycle, "tool_call_count", len(calls), "finish_reason", response.FinishReason())
		if len(calls) == 0 {
			next, nextMessages, complete, err := e.assessCompletion(ctx, phase, messages, providerTools, response, all, state, true, cycle)
			if err != nil || complete {
				return next, nextMessages, all, err
			}
			response, messages = next, nextMessages
			continue
		}
		if response.FinishReason() == "length" {
			return llm.LlmResponse{}, nil, nil, fmt.Errorf("agent response was truncated")
		}
		if err := phase.Transition(turn.PhasePlan); err != nil {
			return llm.LlmResponse{}, nil, nil, err
		}
		batchCalls := make([]execution.Call, 0, len(calls))
		for _, call := range calls {
			candidate := execution.FromLLM(call)
			if _, duplicate := e.Observations.Full(candidate.ID); duplicate {
				return llm.LlmResponse{}, nil, nil, fmt.Errorf("invalid agent tool call: duplicate tool call ID across turn: %s", candidate.ID)
			}
			batchCalls = append(batchCalls, candidate)
		}
		if err := phase.Transition(turn.PhaseGate); err != nil {
			return llm.LlmResponse{}, nil, nil, err
		}
		observability.Info(ctx, "agent.tool.batch_started", "cycle", cycle, "tool_call_count", len(batchCalls))
		batch, err := e.executeBatch(ctx, executor, hook, state, batchCalls)
		if err != nil {
			return llm.LlmResponse{}, nil, nil, err
		}
		if err := phase.Transition(turn.PhaseExecute); err != nil {
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
		messages, err = appendRegistryProtocol(messages, response, batch, e.Observations, e.Registry, e.Agent)
		if err != nil {
			return llm.LlmResponse{}, nil, nil, err
		}
		if err := phase.Transition(turn.PhaseObserve); err != nil {
			return llm.LlmResponse{}, nil, nil, err
		}
		providerTools = availableProviderTools(providerTools, executor.Ledger)

		tools := providerTools
		stage := fmt.Sprintf("llm.tool_follow_up.%d", cycle)
		continueTools := e.recoveryDecision(batch, state.RemainingSteps()) == turn.RecoveryRetryModel
		if !continueTools || !state.AdvanceStep() {
			// Terminal synthesis has an independent reserve so observations from
			// the last legal tool round are never discarded.
			if err := phase.Transition(turn.PhaseFinalize); err != nil {
				return llm.LlmResponse{}, nil, nil, err
			}
			tools = nil
			stage = "llm.terminal_synthesis"
		} else if err := phase.Transition(turn.PhaseModel); err != nil {
			return llm.LlmResponse{}, nil, nil, err
		}
		nextRequest, err := e.projectRequest(messages, tools)
		if err != nil {
			return llm.LlmResponse{}, nil, nil, err
		}
		response, err = e.Client.Complete(observability.WithStage(ctx, stage), nextRequest)
		if err != nil {
			return llm.LlmResponse{}, nil, nil, err
		}
		if err := ctx.Err(); err != nil {
			return llm.LlmResponse{}, nil, nil, err
		}
		if phase.Phase() == turn.PhaseFinalize {
			final, finalMessages, complete, err := e.assessCompletion(ctx, phase, messages, providerTools, response, all, state, false, cycle)
			if err != nil || complete {
				return final, finalMessages, all, err
			}
			return llm.LlmResponse{}, nil, all, fmt.Errorf("terminal synthesis unexpectedly continued")
		}
	}
}

func (e TurnEngine) assessCompletion(ctx context.Context, phase *turn.PhaseMachine, messages []llm.LlmMessage, providerTools []any, response llm.LlmResponse, all []toolBatchResult, state *turn.State, continuationAllowed bool, cycle int) (llm.LlmResponse, []llm.LlmMessage, bool, error) {
	if err := phase.Transition(turn.PhaseReflect); err != nil {
		return llm.LlmResponse{}, nil, false, err
	}
	results := make([]contract.Result, 0, len(all))
	calls := make([]turn.ToolCallEvidence, 0, len(all))
	for _, item := range all {
		results = append(results, item.Result)
		call := turn.ToolCallEvidence{
			CallID: item.Call.ID, Tool: item.Call.Name,
			Arguments: string(contract.CanonicalJSON(item.Call.Arguments)),
		}
		if registered, found := e.Registry.Lookup(item.Call.Name); found {
			if descriptor, descriptorErr := registered.Descriptor(e.Agent); descriptorErr == nil {
				call.Effect = descriptor.Effect()
				call.ResultMode = descriptor.ResultMode()
			}
		}
		calls = append(calls, call)
	}
	observability.Info(ctx, "agent.completion_gate.started", "cycle", cycle, "candidate_chars", len([]rune(response.Content())), "observation_count", len(results))
	assessment, err := e.Gate.Evaluate(ctx, turn.CompletionCandidate{
		Request:      e.Prompt,
		Answer:       response.Content(),
		CanContinue:  continuationAllowed && state.RemainingSteps() > 0,
		Conversation: append([]llm.LlmMessage(nil), messages...),
		Tools:        completionToolCapabilities(providerTools),
		Calls:        calls,
		Results:      results,
	})
	if err != nil {
		return llm.LlmResponse{}, nil, false, fmt.Errorf("semantic completion gate: %w", err)
	}
	if assessment.ReplyMode != turn.CompletionSend && assessment.ReplyMode != turn.CompletionSuppress {
		return llm.LlmResponse{}, nil, false, fmt.Errorf("semantic completion gate returned invalid reply mode %q", assessment.ReplyMode)
	}
	if assessment.Decision == turn.CompletionContinue && assessment.ReplyMode != turn.CompletionSend {
		return llm.LlmResponse{}, nil, false, fmt.Errorf("semantic completion gate suppressed an unfinished response")
	}
	observability.Info(ctx, "agent.completion_gate.completed", "cycle", cycle, "decision", string(assessment.Decision), "feedback_chars", len([]rune(assessment.Feedback)))
	if assessment.Decision == turn.CompletionFinal {
		if assessment.ReplyMode == turn.CompletionSuppress {
			if !canSuppressCompletedReply(e.Registry, e.Agent, all) {
				return llm.LlmResponse{}, nil, false, fmt.Errorf("semantic completion gate suppressed a response without verified room delivery")
			}
			if e.Control != nil {
				e.Control.SuppressReply = true
			}
		}
		final, finalMessages, err := e.finalize(ctx, phase, messages, response)
		return final, finalMessages, true, err
	}
	if assessment.Decision != turn.CompletionContinue {
		return llm.LlmResponse{}, nil, false, fmt.Errorf("semantic completion gate returned invalid decision %q", assessment.Decision)
	}
	if !continuationAllowed || !state.AdvanceStep() {
		return llm.LlmResponse{}, nil, false, fmt.Errorf("semantic completion remains unsatisfied at step limit")
	}
	feedback, err := json.Marshal(map[string]string{"decision": string(assessment.Decision), "feedback": assessment.Feedback})
	if err != nil {
		return llm.LlmResponse{}, nil, false, fmt.Errorf("encode semantic completion feedback: %w", err)
	}
	messages = append(messages,
		llm.NewLlmMessage("assistant", response.Content(), nil, ""),
		llm.NewLlmMessage("user", "SEMANTIC_COMPLETION_FEEDBACK="+string(feedback), nil, ""),
	)
	if err := phase.Transition(turn.PhaseModel); err != nil {
		return llm.LlmResponse{}, nil, false, err
	}
	observability.Info(ctx, "agent.correction.started", "correction_type", "semantic_completion", "cycle", cycle)
	request, err := e.projectRequest(messages, providerTools)
	if err != nil {
		return llm.LlmResponse{}, nil, false, err
	}
	next, err := e.Client.Complete(observability.WithStage(ctx, fmt.Sprintf("llm.semantic_completion_retry.%d", cycle)), request)
	if err != nil {
		return llm.LlmResponse{}, nil, false, err
	}
	if err := ctx.Err(); err != nil {
		return llm.LlmResponse{}, nil, false, err
	}
	return next, messages, false, nil
}

func canSuppressCompletedReply(registry *tool.Registry, agent api.Context, batch []toolBatchResult) bool {
	if registry == nil || len(batch) == 0 {
		return false
	}
	hasVerifiedDelivery := false
	for _, item := range batch {
		if item.Result.IsError {
			return false
		}
		registered, ok := registry.Lookup(item.Call.Name)
		if !ok {
			return false
		}
		descriptor, err := registered.Descriptor(agent)
		if err != nil {
			return false
		}
		if descriptor.Effect() == contract.Action && descriptor.ResultMode() != contract.RoomDelivery {
			return false
		}
		if descriptor.ResultMode() == contract.RoomDelivery {
			if !item.Result.VerifiedRoomDelivery() {
				return false
			}
			hasVerifiedDelivery = true
		}
	}
	return hasVerifiedDelivery
}

// ResumeAction executes one paused action after validating the opaque resume
// token and re-running the current registry, schema, and capability checks.
// A stale authorization therefore becomes a structured observation rather
// than allowing a previously approved side effect to escape current policy.
func (e TurnEngine) ResumeAction(ctx context.Context, pending turn.PendingAction, resumeToken string, current api.Context) (contract.Result, error) {
	call := pending.Request.Call
	if strings.TrimSpace(pending.ResumeToken) == "" || subtle.ConstantTimeCompare([]byte(pending.ResumeToken), []byte(strings.TrimSpace(resumeToken))) != 1 {
		return contract.Result{}, fmt.Errorf("invalid action resume token")
	}
	if err := execution.ValidateBatchIdentity([]execution.Call{call}); err != nil {
		return contract.Result{}, fmt.Errorf("invalid pending action: %w", err)
	}
	if pending.Request.Descriptor.Name() != call.Name || pending.Request.Descriptor.IsReadOnly() {
		return contract.Result{}, fmt.Errorf("pending action contract does not match its call")
	}
	executor := &execution.Executor{
		Registry:       e.Registry,
		Ledger:         execution.NewLedger(toolBudgets(e.Allowed, e.Limits), normalizedFailureLimit(e.Limits)),
		DefaultTimeout: e.Limits.ToolTimeout,
	}
	return executor.Execute(ctx, current, call), nil
}

func (e TurnEngine) recoveryDecision(batch []toolBatchResult, roundsRemaining int) turn.RecoveryDecision {
	decision := turn.RecoveryRetryModel
	for _, item := range batch {
		if !item.Result.IsError {
			continue
		}
		effect := contract.ReadOnly
		idempotent := true
		if registered, ok := e.Registry.Lookup(item.Call.Name); ok {
			if descriptor, err := registered.Descriptor(e.Agent); err == nil {
				effect = descriptor.Effect()
				idempotent = descriptor.Idempotent()
			}
		}
		candidate := e.Recovery.Decide(turn.RecoveryInput{
			ErrorCode:           item.Result.ErrorCode,
			Effect:              effect,
			Idempotent:          idempotent,
			ToolRoundsRemaining: roundsRemaining,
			HasObservations:     true,
		})
		if candidate != turn.RecoveryRetryModel {
			return candidate
		}
	}
	return decision
}

func (e TurnEngine) executeBatch(ctx context.Context, executor *execution.Executor, hook turn.InterruptHook, state *turn.State, calls []execution.Call) ([]toolBatchResult, error) {
	if len(calls) > e.Limits.MaxToolCalls || !state.ReserveToolCalls(len(calls)) {
		return nil, fmt.Errorf("agent tool call limit")
	}
	if err := execution.ValidateBatchIdentity(calls); err != nil {
		return nil, fmt.Errorf("invalid agent tool call: %w", err)
	}
	if err := state.MarkToolAttempted(len(calls)); err != nil {
		return nil, err
	}
	denied := make(map[string]contract.Result)
	executable := make([]execution.Call, 0, len(calls))
	for _, call := range calls {
		registered, ok := e.Registry.Lookup(call.Name)
		if !ok {
			executable = append(executable, call)
			continue
		}
		descriptor, err := registered.Descriptor(e.Agent)
		if err != nil || descriptor.IsReadOnly() {
			executable = append(executable, call)
			continue
		}
		decision, err := hook.Review(ctx, turn.ActionRequest{Call: call, Descriptor: descriptor, Context: e.Agent})
		if err != nil {
			return nil, fmt.Errorf("review tool action %s: %w", call.Name, err)
		}
		switch decision.Outcome {
		case turn.InterruptAllow:
			executable = append(executable, call)
		case turn.InterruptDeny:
			denied[call.ID] = contract.ErrorResult(call.ID, call.Name, "ACTION_DENIED", decision.Reason)
		case turn.InterruptPause:
			return nil, &turn.PausedError{Pending: turn.PendingAction{Request: turn.ActionRequest{Call: call, Descriptor: descriptor, Context: e.Agent}, ResumeToken: decision.ResumeToken, Reason: decision.Reason}}
		default:
			return nil, fmt.Errorf("invalid interrupt outcome for %s", call.Name)
		}
	}
	executed := execution.ExecuteAll(ctx, executor, e.Agent, executable)
	byID := make(map[string]contract.Result, len(executed)+len(denied))
	for index, call := range executable {
		byID[call.ID] = executed[index]
	}
	for id, result := range denied {
		byID[id] = result
	}
	batch := make([]toolBatchResult, 0, len(calls))
	for _, call := range calls {
		result := byID[call.ID]
		if result.IsError {
			_ = state.RecordToolFailure()
		} else {
			_ = state.RecordToolSuccess()
		}
		batch = append(batch, toolBatchResult{Call: call, Result: result})
	}
	return batch, nil
}

func (e TurnEngine) finalize(ctx context.Context, phase *turn.PhaseMachine, messages []llm.LlmMessage, response llm.LlmResponse) (llm.LlmResponse, []llm.LlmMessage, error) {
	if phase.Phase() != turn.PhaseFinalize {
		if err := phase.Transition(turn.PhaseFinalize); err != nil {
			return llm.LlmResponse{}, nil, err
		}
	}
	if err := e.Final.Validate(turn.FinalResponseInput{Response: response}); err != nil {
		if transitionErr := phase.Transition(turn.PhaseReflect); transitionErr != nil {
			return llm.LlmResponse{}, nil, err
		}
		reflectionMessages := append([]llm.LlmMessage(nil), messages...)
		reflectionMessages = append(reflectionMessages,
			llm.NewLlmMessage("assistant", response.Content(), response.ToolCalls(), ""),
			llm.NewLlmMessage("user", "Produce one complete final answer to the newest request using the available observations. Do not call tools, repeat an earlier answer, or return an empty response.", nil, ""),
		)
		request, projectionErr := e.projectRequest(reflectionMessages, nil)
		if projectionErr != nil {
			return llm.LlmResponse{}, nil, projectionErr
		}
		corrected, correctionErr := e.Client.Complete(observability.WithStage(ctx, "llm.final_reflection"), request)
		if correctionErr != nil {
			return llm.LlmResponse{}, nil, correctionErr
		}
		if transitionErr := phase.Transition(turn.PhaseFinalize); transitionErr != nil {
			return llm.LlmResponse{}, nil, transitionErr
		}
		if err := e.Final.Validate(turn.FinalResponseInput{Response: corrected, PriorAssistant: strings.TrimSpace(response.Content())}); err != nil {
			return llm.LlmResponse{}, nil, fmt.Errorf("invalid final response after reflection: %w", err)
		}
		response = corrected
		messages = reflectionMessages
	}
	if err := phase.Transition(turn.PhaseComplete); err != nil {
		return llm.LlmResponse{}, nil, err
	}
	return response, messages, nil
}

func (e TurnEngine) projectRequest(messages []llm.LlmMessage, tools []any) (llm.LlmRequest, error) {
	projection, err := e.Projector.ProjectTurn(messages, tools, e.Observations, e.NewestRequest)
	if err != nil {
		return llm.LlmRequest{}, fmt.Errorf("project agent turn: %w", err)
	}
	return llm.NewLlmRequest(projection.Messages, tools, false, nil, projection), nil
}

func availableProviderTools(providerTools []any, ledger *execution.Ledger) []any {
	if ledger == nil {
		return append([]any(nil), providerTools...)
	}
	available := make([]any, 0, len(providerTools))
	for _, raw := range providerTools {
		definition, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		function, ok := definition["function"].(map[string]any)
		if !ok {
			continue
		}
		name, _ := function["name"].(string)
		if ledger.Available(name) {
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
	return budgets
}

func normalizedFailureLimit(limits turn.ExecutionLimits) int {
	if limits.MaxToolFailures > 0 {
		return limits.MaxToolFailures
	}
	return 2
}
