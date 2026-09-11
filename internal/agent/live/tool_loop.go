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
	"zenbot/internal/agent/turn"
)

const userMessageHistoryTool = "user_message_history"
const roomUsersTool = "room_users"

// ToolLoop holds immutable composition. Every invocation gets a fresh execution
// ledger, observation store, retrieval tool and transcript.
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
	Observations    *assemble.ObservationStore
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
func (l ToolLoop) CompleteWithEvidenceAndHistorical(ctx context.Context, inv runtime.Invocation, memory []llm.LlmMessage, recent string, historical []turn.HistoricalEvidence) (completion Completion, err error) {
	started := time.Now()
	state := turn.NewState(l.Limits)
	defer func() {
		evidence := state.Evidence()
		completion.ToolAttempted = evidence.Attempted
		attributes := []any{"duration_ms", time.Since(started).Milliseconds(), "tool_attempt_count", evidence.AttemptedCount, "tool_success_count", evidence.SuccessfulCount, "tool_failure_count", evidence.FailedCount}
		if err != nil {
			observability.Error(ctx, "agent.loop.failed", err, attributes...)
		} else {
			observability.Info(ctx, "agent.loop.completed", attributes...)
		}
	}()
	if err := ctx.Err(); err != nil {
		return completion, err
	}
	if l.Assembler == nil || l.Client == nil || l.Registry == nil {
		return completion, errors.New("agent tool loop is not initialized")
	}
	if l.Limits.MaxSteps < 1 || l.Limits.MaxToolCalls < 1 {
		return completion, errors.New("agent tool loop limits are insufficient")
	}
	caps := make([]api.Capability, 0, len(inv.Context().Capabilities()))
	for _, capability := range inv.Context().Capabilities() {
		caps = append(caps, api.Capability(capability))
	}
	users := inv.Context().RoomUsers()
	if users == nil {
		users = []string{}
	}
	agent, err := api.NewContextWithCapabilities(inv.Context().Room(), inv.Context().Nick(), inv.Context().Trip(), inv.Context().Hash(), inv.Context().Whisper(), users, caps, inv.Context().ModerationTarget())
	if err != nil {
		return completion, fmt.Errorf("tool invocation context: %w", err)
	}

	observations := assemble.NewObservationStore()
	completion.Observations = observations
	requestTools := make([]tool.Tool, 0, len(l.allowed)+1)
	for _, name := range l.allowed {
		if registered, found := l.Registry.Lookup(name); found {
			requestTools = append(requestTools, registered)
		}
	}
	requestTools = append(requestTools, NewReadToolResult(observations))
	allowed := append(append([]string(nil), l.allowed...), readToolResultName)
	registry := tool.NewRegistry(requestTools, allowed)
	var definitions []any
	if !agent.Whisper() {
		definitions, err = providerToolDefinitions(registry, agent)
		if err != nil {
			return completion, err
		}
	}
	prepared, err := l.Assembler.AssembleWithHistoricalEvidence(ctx, inv, memory, recent, definitions, assemble.Talk, historical)
	if err != nil {
		return completion, fmt.Errorf("assemble agent request: %w", err)
	}
	state.AdvanceStep()
	initial := prepared.LlmRequest()
	first, err := l.Client.Complete(observability.WithStage(ctx, "llm.initial"), initial)
	if err != nil {
		return completion, fmt.Errorf("complete agent request: %w", err)
	}
	control := &CompletionControl{}
	engine := TurnEngine{
		Client: l.Client, Registry: registry, Agent: agent, Projector: l.Assembler,
		NewestRequest: llm.NewLlmMessage("user", prepared.ContextualizedPrompt(), nil, ""),
		Observations:  observations, Allowed: allowed, Limits: l.Limits, Prompt: inv.Prompt(),
		NoReplyMarker: l.Assembler.NoReplyMarker(), AllowSilence: !inv.Mode().RequiresReply(), Control: control,
	}
	response, _, batch, loopErr := engine.Complete(ctx, initial.Messages(), initial.Tools(), first, state)
	completion.Response, completion.SuppressReply = response, control.SuppressReply
	if completion.SuppressReply {
		// The marker controls delivery, but memory needs the actual delivered
		// observations to support later follow-ups.
		var delivered []string
		for _, item := range batch {
			if item.Result.VerifiedRoomDelivery() {
				view, _ := observations.View(item.Call.ID)
				delivered = append(delivered, item.Call.Name+": "+string(view.Data))
			}
		}
		completion.Response = llm.NewLlmResponse(strings.Join(delivered, "\n"), nil, "stop")
	}
	for _, item := range batch {
		registered, found := registry.Lookup(item.Call.Name)
		if !found {
			continue
		}
		descriptor, descriptorErr := registered.Descriptor(agent)
		if descriptorErr != nil {
			continue
		}
		evidence, evidenceErr := turn.NewPersistableEvidence(descriptor, item.Result)
		if evidenceErr == nil {
			completion.DurableEvidence = append(completion.DurableEvidence, evidence)
		}
	}
	// Return collected evidence even if a later provider call or projection fails.
	return completion, loopErr
}

// NewRegistryToolLoop composes the single iterative model/tool implementation.
func NewRegistryToolLoop(assembler *assemble.Assembler, client llm.LlmClient, tools []tool.Tool, allowed []string, limits turn.ExecutionLimits) (*ToolLoop, error) {
	if assembler == nil || client == nil || len(tools) == 0 || len(tools) != len(allowed) {
		return nil, errors.New("agent tool loop is incomplete")
	}
	if limits.MaxSteps < 1 || limits.MaxToolCalls < 1 {
		return nil, errors.New("agent tool loop limits are insufficient")
	}
	seen := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		if strings.TrimSpace(name) == "" || seen[name] || name == readToolResultName {
			return nil, errors.New("invalid tool inventory")
		}
		seen[name] = true
	}
	ctx, err := api.NewContext("room", "agent", "", "", false, []string{})
	if err != nil {
		return nil, err
	}
	toolNames := make(map[string]bool, len(tools))
	for _, registered := range tools {
		if registered == nil || !seen[registered.Name()] || toolNames[registered.Name()] {
			return nil, errors.New("invalid tool inventory")
		}
		descriptor, err := registered.Descriptor(ctx)
		if err != nil {
			return nil, fmt.Errorf("tool %q descriptor: %w", registered.Name(), err)
		}
		if descriptor.Name() != registered.Name() {
			return nil, fmt.Errorf("tool %q descriptor identity mismatch", registered.Name())
		}
		toolNames[registered.Name()] = true
	}
	registry := tool.NewRegistry(append([]tool.Tool(nil), tools...), append([]string(nil), allowed...))
	definitions, err := providerToolDefinitions(registry, ctx)
	if err != nil {
		return nil, err
	}
	return &ToolLoop{Assembler: assembler, Client: client, Registry: registry, Tools: definitions, allowed: append([]string(nil), allowed...), Limits: limits}, nil
}

func providerToolDefinitions(registry *tool.Registry, ctx api.Context) ([]any, error) {
	manifest, err := registry.Manifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("build tool manifest: %w", err)
	}
	definitions := manifest.ProviderDefinitions()
	out := make([]any, 0, len(definitions))
	for _, definition := range definitions {
		value, err := providerToolDefinition(definition)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}
func providerToolDefinition(definition contract.Definition) (any, error) {
	var parameters any
	if err := json.Unmarshal(definition.Parameters, &parameters); err != nil {
		return nil, err
	}
	return map[string]any{"type": "function", "function": map[string]any{
		"name": definition.Name, "description": definition.Description, "parameters": parameters,
		"strict": contract.SupportsStrictParameters(definition.Parameters),
	}}, nil
}
