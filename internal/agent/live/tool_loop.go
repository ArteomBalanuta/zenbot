package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/observability"
	"zenbot/internal/agent/participation"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
	"zenbot/internal/agent/turn"
)

const userMessageHistoryTool = "user_message_history"

const compactRetryHistoryMessages = 4

// ToolLoop owns one request-local, bounded tool turn. Its registry, executor,
// and provider are frozen at composition; it never accepts model-selected tools.
type ToolLoop struct {
	Assembler *assemble.Assembler
	Client    llm.LlmClient
	Registry  *tool.Registry
	Tools     []any
	allowed   []string
	general   bool
	Limits    turn.ExecutionLimits
	Interrupt turn.InterruptHook
	Gate      turn.CompletionGate
}

func ToolLoopLimits() turn.ExecutionLimits { return turn.ExecutionLimits{MaxSteps: 2, MaxToolCalls: 1} }

type Completion struct {
	Response        llm.LlmResponse
	DurableEvidence []turn.PersistableEvidence
	SuppressReply   bool
	CandidateKind   participation.RequestKind
	ToolAttempted   bool
}

func (c Completion) Evidence() []turn.PersistableEvidence {
	return append([]turn.PersistableEvidence(nil), c.DurableEvidence...)
}
func (l ToolLoop) Complete(ctx context.Context, inv runtime.Invocation, memory []llm.LlmMessage, recent string) (llm.LlmResponse, error) {
	c, err := l.CompleteWithEvidence(ctx, inv, memory, recent)
	return c.Response, err
}
func (l ToolLoop) CompleteWithEvidence(ctx context.Context, inv runtime.Invocation, memory []llm.LlmMessage, recent string) (Completion, error) {
	return l.CompleteWithEvidenceAndHistorical(ctx, inv, memory, recent, nil)
}

// CompleteWithEvidenceAndHistorical projects durable evidence only on completion #1.
func (l ToolLoop) CompleteWithEvidenceAndHistorical(ctx context.Context, inv runtime.Invocation, memory []llm.LlmMessage, recent string, historical []turn.HistoricalEvidence) (completion Completion, err error) {
	candidateKind := participation.Classifier{}.Classify(inv.Prompt())
	started := time.Now()
	var state *turn.State
	observability.Info(ctx, "agent.loop.started",
		"candidate_kind", string(candidateKind),
		"max_steps", l.Limits.MaxSteps,
		"max_tool_calls", l.Limits.MaxToolCalls,
		"memory_turn_count", len(memory),
		"historical_evidence_count", len(historical),
		"recent_context_bytes", len(recent),
	)
	defer func() {
		completion.CandidateKind = candidateKind
		attributes := []any{"duration_ms", time.Since(started).Milliseconds(), "candidate_kind", string(candidateKind)}
		if state != nil {
			evidence := state.Evidence()
			completion.ToolAttempted = evidence.Attempted
			attributes = append(attributes,
				"tool_attempt_count", evidence.AttemptedCount,
				"tool_success_count", evidence.SuccessfulCount,
				"tool_failure_count", evidence.FailedCount,
			)
		}
		if err != nil {
			observability.Error(ctx, "agent.loop.failed", err, attributes...)
		} else {
			observability.Info(ctx, "agent.loop.completed", attributes...)
		}
	}()
	if err := ctx.Err(); err != nil {
		return Completion{}, err
	}
	if l.Assembler == nil || l.Client == nil || l.Registry == nil {
		return Completion{}, errors.New("agent tool loop is not initialized")
	}
	if l.Limits.MaxSteps < 2 || l.Limits.MaxToolCalls < 1 {
		return Completion{}, errors.New("agent tool loop limits are insufficient")
	}
	users := inv.Context().RoomUsers()
	if users == nil {
		users = []string{}
	}
	caps := make([]api.Capability, 0, len(inv.Context().Capabilities()))
	for _, capability := range inv.Context().Capabilities() {
		caps = append(caps, api.Capability(capability))
	}
	agent, err := api.NewContextWithCapabilities(inv.Context().Room(), inv.Context().Nick(), inv.Context().Trip(), inv.Context().Hash(), inv.Context().Whisper(), users, caps, inv.Context().ModerationTarget())
	if err != nil {
		return Completion{}, fmt.Errorf("tool invocation context: %w", err)
	}
	definitions := []any(nil)
	if !inv.Context().Whisper() {
		if l.general {
			definitions, err = providerToolDefinitions(l.Registry, agent)
			if err != nil {
				return Completion{}, err
			}
		} else {
			definitions = append([]any(nil), l.Tools...)
		}
	}
	prepared, err := l.Assembler.AssembleWithHistoricalEvidence(ctx, inv, memory, recent, definitions, assemble.Talk, historical)
	if err != nil {
		return Completion{}, fmt.Errorf("assemble agent request: %w", err)
	}
	observability.Info(ctx, "agent.request.assembled",
		"context_items", len(prepared.Messages()),
		"tool_definition_count", len(prepared.Tools()),
	)
	state = turn.NewState(l.Limits)
	if !state.AdvanceStep() {
		return Completion{}, errors.New("agent tool loop step limit")
	}
	initialRequest := prepared.LlmRequest()
	providerTools := initialRequest.Tools()
	newestRequest := llm.NewLlmMessage("user", prepared.ContextualizedPrompt(), nil, "")
	observations := assemble.NewObservationStore()
	first, err := l.Client.Complete(observability.WithStage(ctx, "llm.initial"), initialRequest)
	if err != nil {
		return Completion{}, fmt.Errorf("complete agent request: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Completion{}, err
	}
	if l.general && first.FinishReason() == "length" && len(first.ToolCalls()) == 0 {
		if !state.AdvanceStep() {
			return Completion{}, errors.New("agent response was truncated and no compact retry remained")
		}
		observability.Info(ctx, "agent.correction.started", "correction_type", "compact_truncation_retry")
		first, err = l.Client.Complete(observability.WithStage(ctx, "llm.compact_truncation_retry"), compactInitialRetryRequest(initialRequest))
		if err != nil {
			return Completion{}, fmt.Errorf("complete compact truncation retry: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return Completion{}, err
		}
		if first.FinishReason() == "length" {
			return Completion{}, errors.New("agent response remained truncated after compact retry")
		}
	}
	if first.FinishReason() == "length" && !l.general {
		return Completion{}, errors.New("agent response was truncated")
	}
	if l.general {
		response, _, batch, suppressReply, loopErr := completeRegistryLoop(ctx, l.Client, l.Registry, agent, l.Assembler, newestRequest, observations, initialRequest.Messages(), providerTools, first, l.allowed, l.Limits, state, l.Interrupt, l.Gate, inv.Prompt())
		if loopErr != nil {
			return Completion{}, loopErr
		}
		evidence := make([]turn.PersistableEvidence, 0, len(batch))
		for _, item := range batch {
			registered, ok := l.Registry.Lookup(item.Call.Name)
			if !ok {
				continue
			}
			descriptor, descriptorErr := registered.Descriptor(agent)
			if descriptorErr != nil {
				continue
			}
			candidate, candidateErr := turn.NewPersistableEvidence(descriptor, item.Result)
			if candidateErr == nil {
				evidence = append(evidence, candidate)
			}
		}
		return Completion{
			Response:        response,
			DurableEvidence: evidence,
			SuppressReply:   suppressReply,
		}, nil
	}
	calls := first.ToolCalls()
	if len(calls) == 0 {
		if !containsAllowed(l.allowed, "run_command") {
			return Completion{Response: first}, nil
		}
		channel, channelErr := newCommandChannel(agent, l.Client, l.Registry, l.allowed, l.Tools)
		if channelErr != nil {
			return Completion{}, channelErr
		}
		if command, found := channel.guard.FindCommand(first.Content()); found {
			if inv.Context().Whisper() {
				return Completion{}, errors.New("rendered commands are not permitted for whispers")
			}
			return channel.correct(ctx, inv, agent, prepared.Messages(), first, command, state)
		}
		return Completion{Response: first}, nil
	}
	if inv.Context().Whisper() {
		return Completion{}, errors.New("tool calls are not permitted for whispers")
	}
	if len(calls) != 1 || strings.TrimSpace(calls[0].ID()) == "" {
		return Completion{}, errors.New("invalid bounded tool call")
	}
	call := execution.FromLLM(calls[0])
	registered, ok := l.Registry.Lookup(call.Name)
	if !ok || !l.Registry.Allowed(call.Name) {
		return Completion{}, errors.New("unknown bounded tool call")
	}
	descriptor, err := registered.Descriptor(agent)
	if err != nil || contract.ValidateArguments(descriptor.Parameters(), call.Arguments) != nil {
		return Completion{}, errors.New("invalid bounded tool arguments")
	}
	if !state.ReserveToolCalls(1) {
		return Completion{}, errors.New("agent tool call limit")
	}
	if err := state.MarkToolAttempted(1); err != nil {
		return Completion{}, err
	}
	limits := make(map[string]int, len(l.allowed))
	for _, name := range l.allowed {
		limits[name] = 1
	}
	executor := &execution.Executor{Registry: l.Registry, Ledger: execution.NewLedger(limits, 1)}
	result := executor.Execute(ctx, agent, call)
	if err := ctx.Err(); err != nil {
		return Completion{}, err
	}
	if result.IsError {
		_ = state.RecordToolFailure()
		if call.Name == "run_command" {
			return Completion{}, errors.New("run command failed")
		}
	} else {
		_ = state.RecordToolSuccess()
	}
	var assistantContent any
	if first.ContentNullable() != nil {
		assistantContent = first.Content()
	}
	messages := append(prepared.Messages(), llm.NewLlmMessage("assistant", assistantContent, calls, ""))
	messages = append(messages, llm.NewLlmMessage("tool", string(result.Envelope()), nil, call.ID))
	if !state.AdvanceStep() {
		return Completion{}, errors.New("agent tool loop step limit")
	}
	second, err := l.Client.Complete(observability.WithStage(ctx, "llm.bounded_tool_follow_up"), llm.NewLlmRequest(messages, nil, false, nil, nil))
	if err != nil {
		return Completion{}, fmt.Errorf("complete agent tool follow-up: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Completion{}, err
	}
	if second.FinishReason() == "length" || len(second.ToolCalls()) != 0 || strings.TrimSpace(second.Content()) == "" {
		return Completion{}, errors.New("invalid bounded tool follow-up")
	}
	if call.Name == "run_command" {
		channel, channelErr := newCommandChannel(agent, l.Client, l.Registry, l.allowed, l.Tools)
		if channelErr != nil {
			return Completion{}, channelErr
		}
		if channel.guardHasCommand(second.Content()) {
			return Completion{SuppressReply: true}, nil
		}
	}
	candidate, _ := turn.NewPersistableEvidence(descriptor, result)
	return Completion{Response: second, DurableEvidence: func() []turn.PersistableEvidence {
		if candidate.Tool == "" {
			return nil
		}
		return []turn.PersistableEvidence{candidate}
	}()}, nil
}

func compactInitialRetryRequest(request llm.LlmRequest) llm.LlmRequest {
	source := request.Messages()
	if len(source) <= 2 {
		return llm.NewLlmRequest(source, request.Tools(), true, request.ResponseFormat(), nil)
	}

	compact := make([]llm.LlmMessage, 0, compactRetryHistoryMessages+2)
	start := 0
	if source[0].Role() == "system" {
		compact = append(compact, source[0])
		start = 1
	}
	latest := source[len(source)-1]
	history := make([]llm.LlmMessage, 0, compactRetryHistoryMessages)
	for _, message := range source[start : len(source)-1] {
		content := strings.TrimSpace(message.Content())
		if (message.Role() != "user" && message.Role() != "assistant") || len(message.ToolCalls()) != 0 || message.ToolCallID() != "" || content == "" {
			continue
		}
		if isBulkRetryContext(content) {
			continue
		}
		history = append(history, message)
	}
	if len(history) > compactRetryHistoryMessages {
		history = history[len(history)-compactRetryHistoryMessages:]
	}
	compact = append(compact, history...)
	compact = append(compact, latest)
	return llm.NewLlmRequest(compact, request.Tools(), true, request.ResponseFormat(), nil)
}

func isBulkRetryContext(content string) bool {
	for _, prefix := range []string{
		"RECENT_PUBLIC_ROOM_MESSAGES_UNTRUSTED_DATA=",
		"HISTORICAL_TOOL_EVIDENCE_UNTRUSTED_DATA=",
		"UNTRUSTED_CONVERSATION_SUMMARY ",
		"[Internal tool evidence",
		"SEMANTIC_COMPLETION_FEEDBACK=",
	} {
		if strings.HasPrefix(content, prefix) {
			return true
		}
	}
	return false
}

// NewBoundedToolLoop freezes exactly three public tools: two reads and one ordered command action.
func NewBoundedToolLoop(assembler *assemble.Assembler, client llm.LlmClient, tools []tool.Tool, allowed []string) (*ToolLoop, error) {
	if len(tools) != 3 || len(allowed) != 3 || !containsExactly(allowed, userMessageHistoryTool, roomUsersTool, "run_command") || !frozenPublicTools(tools) {
		return nil, errors.New("bounded tool loop requires fixed history, room users, and run command tools")
	}
	return newFrozenToolLoop(assembler, client, tools, allowed, true)
}

func NewRegistryToolLoop(assembler *assemble.Assembler, client llm.LlmClient, gate turn.CompletionGate, tools []tool.Tool, allowed []string, limits turn.ExecutionLimits) (*ToolLoop, error) {
	loop, err := newFrozenToolLoop(assembler, client, tools, allowed, false)
	if err != nil {
		return nil, err
	}
	if gate == nil || limits.MaxSteps < 2 || limits.MaxToolCalls < 1 {
		return nil, errors.New("registry tool loop limits are insufficient")
	}
	loop.Limits, loop.general, loop.Gate = limits, true, gate
	return loop, nil
}

// frozenPublicTools prevents callers from replacing a public tool by reusing
// one of its registry names. The composition is intentionally closed.
func frozenPublicTools(tools []tool.Tool) bool {
	seen := map[string]bool{}
	for _, registered := range tools {
		switch value := registered.(type) {
		case tool.UserMessageHistory:
			if value.Name() != userMessageHistoryTool {
				return false
			}
		case tool.RoomUsers:
			if value.Name() != roomUsersTool {
				return false
			}
		case tool.RunCommand:
			if value.Name() != "run_command" || value.Gateway == nil {
				return false
			}
		default:
			return false
		}
		if seen[registered.Name()] {
			return false
		}
		seen[registered.Name()] = true
	}
	return containsExactly([]string{tools[0].Name(), tools[1].Name(), tools[2].Name()}, userMessageHistoryTool, roomUsersTool, "run_command")
}

// NewHistoryToolLoop remains a compatibility wrapper for existing one-tool callers.
func NewHistoryToolLoop(assembler *assemble.Assembler, client llm.LlmClient, history tool.UserMessageHistory) (*ToolLoop, error) {
	return newFrozenToolLoop(assembler, client, []tool.Tool{history}, []string{userMessageHistoryTool}, true)
}

func newFrozenToolLoop(assembler *assemble.Assembler, client llm.LlmClient, tools []tool.Tool, allowed []string, requirePublicDefinitions bool) (*ToolLoop, error) {
	if assembler == nil || client == nil || len(tools) == 0 || len(tools) != len(allowed) {
		return nil, errors.New("bounded tool loop is incomplete")
	}
	seen := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		if strings.TrimSpace(name) == "" || seen[name] {
			return nil, errors.New("invalid bounded tool inventory")
		}
		seen[name] = true
	}
	toolNames := make(map[string]bool, len(tools))
	for _, registered := range tools {
		if registered == nil || !seen[registered.Name()] || toolNames[registered.Name()] {
			return nil, errors.New("invalid bounded tool inventory")
		}
		toolNames[registered.Name()] = true
	}
	registry := tool.NewRegistry(append([]tool.Tool(nil), tools...), append([]string(nil), allowed...))
	ctx, err := api.NewContext("room", "agent", "", "", false, []string{})
	if err != nil {
		return nil, err
	}
	for _, registered := range tools {
		descriptor, descriptorErr := registered.Descriptor(ctx)
		if descriptorErr != nil {
			return nil, fmt.Errorf("tool %q descriptor: %w", registered.Name(), descriptorErr)
		}
		if descriptor.Name() != registered.Name() {
			return nil, fmt.Errorf("tool %q descriptor identity mismatch", registered.Name())
		}
	}
	manifest, err := registry.Manifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("build tool manifest: %w", err)
	}
	defs := manifest.ProviderDefinitions()
	if requirePublicDefinitions && len(defs) != len(allowed) {
		return nil, errors.New("bounded tool definition is unavailable")
	}
	providerTools := make([]any, 0, len(defs))
	for _, definition := range defs {
		providerDefinition, err := providerToolDefinition(definition)
		if err != nil {
			return nil, err
		}
		providerTools = append(providerTools, providerDefinition)
	}
	return &ToolLoop{Assembler: assembler, Client: client, Registry: registry, Tools: providerTools, allowed: append([]string(nil), allowed...), Limits: ToolLoopLimits()}, nil
}

func providerToolDefinitions(registry *tool.Registry, ctx api.Context) ([]any, error) {
	manifest, err := registry.Manifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("build tool manifest: %w", err)
	}
	defs := manifest.ProviderDefinitions()
	providerTools := make([]any, 0, len(defs))
	for _, definition := range defs {
		providerDefinition, err := providerToolDefinition(definition)
		if err != nil {
			return nil, err
		}
		providerTools = append(providerTools, providerDefinition)
	}
	return providerTools, nil
}

func providerToolDefinition(definition contract.Definition) (any, error) {
	var parameters any
	if err := json.Unmarshal(definition.Parameters, &parameters); err != nil {
		return nil, err
	}
	return map[string]any{"type": "function", "function": map[string]any{"name": definition.Name, "description": definition.Description, "parameters": parameters, "strict": true}}, nil
}

func containsAllowed(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

const roomUsersTool = "room_users"

func containsExactly(values []string, want ...string) bool {
	if len(values) != len(want) {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range want {
		if !seen[value] {
			return false
		}
	}
	return true
}
