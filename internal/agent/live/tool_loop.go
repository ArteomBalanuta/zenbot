package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/participation"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
	"zenbot/internal/agent/turn"
)

const userMessageHistoryTool = "user_message_history"

// ToolLoop owns one request-local, bounded tool turn. Its registry, executor,
// and provider are frozen at composition; it never accepts model-selected tools.
type ToolLoop struct {
	Assembler *assemble.Assembler
	Client    llm.LlmClient
	Registry  *tool.Registry
	Tools     []any
	allowed   []string
	Limits    turn.ExecutionLimits
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
	var state *turn.State
	defer func() {
		completion.CandidateKind = candidateKind
		if state != nil {
			completion.ToolAttempted = state.Evidence().Attempted
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
		definitions = append([]any(nil), l.Tools...)
	}
	prepared, err := l.Assembler.AssembleWithHistoricalEvidence(ctx, inv, memory, recent, definitions, assemble.Talk, historical)
	if err != nil {
		return Completion{}, fmt.Errorf("assemble agent request: %w", err)
	}
	state = turn.NewState(l.Limits)
	if !state.AdvanceStep() {
		return Completion{}, errors.New("agent tool loop step limit")
	}
	first, err := l.Client.Complete(ctx, prepared.LlmRequest())
	if err != nil {
		return Completion{}, fmt.Errorf("complete agent request: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Completion{}, err
	}
	if first.FinishReason() == "length" {
		return Completion{}, errors.New("agent response was truncated")
	}
	if prepared.RequiredFreshTool() != "" {
		return l.completeRequiredHistory(ctx, inv, agent, prepared, first, state)
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
	second, err := l.Client.Complete(ctx, llm.NewLlmRequest(messages, nil, false, nil, nil))
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

// completeRequiredHistory is router-owned: the provider cannot choose its one call.
func (l ToolLoop) completeRequiredHistory(ctx context.Context, inv runtime.Invocation, agent api.Context, prepared assemble.PreparedRequest, first llm.LlmResponse, state *turn.State) (Completion, error) {
	if inv.Context().Whisper() || strings.TrimSpace(agent.Room()) == "" {
		return Completion{}, errors.New("required history is not available for private or roomless invocation")
	}
	if prepared.RequiredFreshTool() != userMessageHistoryTool {
		return Completion{}, errors.New("unsupported required fresh tool")
	}
	nick := turn.NormalizeNick(prepared.RequiredFreshNick())
	if !turn.IsValidNick(nick) {
		return Completion{}, errors.New("invalid required fresh nick")
	}
	registered, ok := l.Registry.Lookup(userMessageHistoryTool)
	if !ok || !l.Registry.Allowed(userMessageHistoryTool) {
		return Completion{}, errors.New("required history tool is unavailable")
	}
	descriptor, err := registered.Descriptor(agent)
	if err != nil || !trustedHistoryDescriptor(descriptor) {
		return Completion{}, errors.New("required history tool contract is invalid")
	}
	args, err := json.Marshal(map[string]string{"nick": nick})
	if err != nil {
		return Completion{}, err
	}
	call := execution.Call{ID: "fresh-history-" + inv.RequestID(), Name: userMessageHistoryTool, Arguments: args}
	if strings.TrimSpace(call.ID) == "fresh-history-" || !state.ReserveToolCalls(1) {
		return Completion{}, errors.New("agent tool call limit")
	}
	if err := state.MarkToolAttempted(1); err != nil {
		return Completion{}, err
	}
	executor := &execution.Executor{Registry: l.Registry, Ledger: execution.NewLedger(map[string]int{userMessageHistoryTool: 1}, 1)}
	result := executor.Execute(ctx, agent, call)
	if err := ctx.Err(); err != nil {
		return Completion{}, err
	}
	if result.IsError {
		_ = state.RecordToolFailure()
		return Completion{}, errors.New("required history lookup failed")
	}
	if err := state.RecordToolSuccess(); err != nil {
		return Completion{}, err
	}
	if !state.RecordSuccessfulTool(userMessageHistoryTool) || state.RecordSuccessfulToolResult(result) != nil {
		return Completion{}, errors.New("required history result is invalid")
	}
	var content any
	if first.ContentNullable() != nil {
		content = first.Content()
	}
	synthetic := llm.NewLlmToolCall(call.ID, userMessageHistoryTool, map[string]any{"nick": nick})
	messages := append(prepared.Messages(), llm.NewLlmMessage("assistant", content, []llm.LlmToolCall{synthetic}, ""))
	messages = append(messages, llm.NewLlmMessage("tool", string(result.Envelope()), nil, call.ID))
	if !state.AdvanceStep() {
		return Completion{}, errors.New("agent tool loop step limit")
	}
	second, err := l.Client.Complete(ctx, llm.NewLlmRequest(messages, nil, false, nil, nil))
	if err != nil {
		return Completion{}, fmt.Errorf("complete required history follow-up: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Completion{}, err
	}
	if second.FinishReason() == "length" || len(second.ToolCalls()) != 0 || strings.TrimSpace(second.Content()) == "" || strings.TrimSpace(second.Content()) == strings.TrimSpace(first.Content()) {
		return Completion{}, errors.New("invalid required history synthesis")
	}
	candidate, _ := turn.NewPersistableEvidence(descriptor, result)
	if candidate.Tool == "" {
		return Completion{Response: second}, nil
	}
	return Completion{Response: second, DurableEvidence: []turn.PersistableEvidence{candidate}}, nil
}

func trustedHistoryDescriptor(d contract.Descriptor) bool {
	return d.Name() == userMessageHistoryTool && d.Access() == contract.AccessUser && d.IsReadOnly() && d.ResultMode() == contract.ModelData && d.Idempotent() && d.Timeout() > 0 && len(d.RequiredCapabilities()) == 0 && len(d.RequiredSuccessfulTools()) == 0 && len(d.ResourceWrites()) == 0 && len(d.ResourceReads()) == 1 && d.ResourceReads()[0] == "messages"
}

// NewBoundedToolLoop freezes exactly three public tools: two reads and one ordered command action.
func NewBoundedToolLoop(assembler *assemble.Assembler, client llm.LlmClient, tools []tool.Tool, allowed []string) (*ToolLoop, error) {
	if len(tools) != 3 || len(allowed) != 3 || !containsExactly(allowed, userMessageHistoryTool, roomUsersTool, "run_command") || !frozenPublicTools(tools) {
		return nil, errors.New("bounded tool loop requires fixed history, room users, and run command tools")
	}
	return newFrozenToolLoop(assembler, client, tools, allowed)
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
	return newFrozenToolLoop(assembler, client, []tool.Tool{history}, []string{userMessageHistoryTool})
}

func newFrozenToolLoop(assembler *assemble.Assembler, client llm.LlmClient, tools []tool.Tool, allowed []string) (*ToolLoop, error) {
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
	defs := registry.Definitions(ctx)
	if len(defs) != len(allowed) {
		return nil, errors.New("bounded tool definition is unavailable")
	}
	providerTools := make([]any, 0, len(defs))
	for _, definition := range defs {
		var parameters any
		if err := json.Unmarshal(definition.Parameters, &parameters); err != nil {
			return nil, err
		}
		providerTools = append(providerTools, map[string]any{"type": "function", "function": map[string]any{"name": definition.Name, "description": definition.Description, "parameters": parameters}})
	}
	return &ToolLoop{Assembler: assembler, Client: client, Registry: registry, Tools: providerTools, allowed: append([]string(nil), allowed...), Limits: ToolLoopLimits()}, nil
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
