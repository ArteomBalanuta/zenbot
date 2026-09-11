package live

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/observability"
	"zenbot/internal/agent/participation"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/agent/turn"
)

type Finalizer interface {
	Finalize(runtime.Invocation, string) (string, bool, error)
}

type FinalizationContext struct {
	CandidateKind participation.RequestKind
	ToolAttempted bool
}

type contextualFinalizer interface {
	FinalizeWithContext(runtime.Invocation, string, FinalizationContext) (string, bool, error)
}

const defaultMaxOutputChars = 8000

// OutputFinalizer deterministically prepares provider text for one visible response.
type OutputFinalizer struct {
	NoReplyMarker  string
	MaxOutputChars int
}

func NewOutputFinalizer(noReplyMarker string, maxOutputChars int) (OutputFinalizer, error) {
	return OutputFinalizer{NoReplyMarker: noReplyMarker, MaxOutputChars: maxOutputChars}, nil
}

func (f OutputFinalizer) Finalize(inv runtime.Invocation, raw string) (string, bool, error) {
	return f.FinalizeWithContext(inv, raw, FinalizationContext{})
}

func (f OutputFinalizer) FinalizeWithContext(inv runtime.Invocation, raw string, meta FinalizationContext) (string, bool, error) {
	content := (responseSanitizer{}).sanitize(raw)
	if stripJavaWhitespace(content) == "" {
		return "", false, fmt.Errorf("agent returned an empty response")
	}
	if stripJavaWhitespace(content) == f.NoReplyMarker {
		if !inv.Mode().RequiresReply() {
			return "", false, nil
		}
		return "", false, fmt.Errorf("agent declined a required response")
	}
	if containsInternalToolEvidence(content) {
		return "", false, fmt.Errorf("agent response exposed internal tool evidence")
	}
	if turn.ContainsToolProtocolArtifact(content) {
		return "", false, fmt.Errorf("agent response exposed tool protocol markup")
	}
	content = trimASCIIControlWhitespace(strings.ReplaceAll(content, f.NoReplyMarker, ""))
	if content == "" {
		return "", false, fmt.Errorf("agent returned an empty response")
	}
	maxOutputChars := f.MaxOutputChars
	if maxOutputChars <= 0 {
		maxOutputChars = defaultMaxOutputChars
	}
	runes := []rune(content)
	if len(runes) > maxOutputChars {
		content = string(runes[:maxOutputChars])
	}
	return content, true, nil
}

func finalizeWithContext(f Finalizer, inv runtime.Invocation, raw string, meta FinalizationContext) (string, bool, error) {
	if contextual, ok := f.(contextualFinalizer); ok {
		return contextual.FinalizeWithContext(inv, raw, meta)
	}
	return f.Finalize(inv, raw)
}

func trimASCIIControlWhitespace(content string) string {
	return strings.Trim(content, " 	\n\r")
}

type MarkerFinalizer struct{ NoReplyMarker string }

func (f MarkerFinalizer) Finalize(inv runtime.Invocation, raw string) (string, bool, error) {
	return OutputFinalizer{NoReplyMarker: f.NoReplyMarker, MaxOutputChars: defaultMaxOutputChars}.Finalize(inv, raw)
}

type Runner struct {
	Assembler           *assemble.Assembler
	Client              llm.LlmClient
	Finalizer           Finalizer
	ConversationContext ConversationContextProvider
	ToolLoop            *ToolLoop
	Memory              *turn.TurnMemory
}

// IncompleteTurnError retains request-local execution evidence when producing
// the final answer fails. Its text never renders result bodies as user output.
type IncompleteTurnError struct {
	RequestID        string
	Completion       Completion
	Cause            error
	PersistenceError error
}

func (failure *IncompleteTurnError) Error() string {
	if failure.PersistenceError != nil {
		return failure.Cause.Error() + "; interrupted execution record could not be retained"
	}
	return failure.Cause.Error()
}

func (failure *IncompleteTurnError) Unwrap() []error {
	if failure.PersistenceError != nil {
		return []error{failure.Cause, failure.PersistenceError}
	}
	return []error{failure.Cause}
}

func (r Runner) Run(ctx context.Context, inv runtime.Invocation) (runtime.Result, error) {
	if err := ctx.Err(); err != nil {
		return runtime.Result{}, err
	}
	if r.Assembler == nil {
		return runtime.Result{}, fmt.Errorf("agent assembler is not initialized")
	}
	if r.Client == nil {
		return runtime.Result{}, fmt.Errorf("agent client is not initialized")
	}
	if r.Finalizer == nil {
		return runtime.Result{}, fmt.Errorf("agent finalizer is not initialized")
	}
	memory, err := r.loadMemory(ctx, inv)
	if err != nil {
		observability.Error(ctx, "agent.context.load_failed", err, "context_source", "memory")
		return runtime.Result{}, err
	}
	historical, err := r.loadHistoricalEvidence(ctx, inv)
	if err != nil {
		observability.Error(ctx, "agent.context.load_failed", err, "context_source", "historical_evidence")
		return runtime.Result{}, err
	}
	recent, err := loadRecentContext(ctx, r.ConversationContext, inv)
	if err != nil {
		observability.Error(ctx, "agent.context.load_failed", err, "context_source", "recent_room")
		return runtime.Result{}, err
	}
	observability.Info(ctx, "agent.context.loaded",
		"memory_turn_count", len(memory),
		"historical_evidence_count", len(historical),
		"recent_context_bytes", len(recent),
	)
	var response llm.LlmResponse
	var completion Completion
	var evidence []turn.PersistableEvidence
	suppressReply := false
	var meta FinalizationContext
	if r.ToolLoop != nil {
		var loopErr error
		completion, loopErr = r.ToolLoop.CompleteWithEvidenceAndHistorical(ctx, inv, memory, recent, historical)
		response, err, evidence, suppressReply = completion.Response, loopErr, completion.Evidence(), completion.SuppressReply
		meta = FinalizationContext{CandidateKind: completion.CandidateKind, ToolAttempted: completion.ToolAttempted}
	} else {
		meta.CandidateKind = participation.Classifier{}.Classify(inv.Prompt())
		prepared, e := r.Assembler.AssembleWithHistoricalEvidence(ctx, inv, memory, recent, nil, assemble.Talk, historical)
		if e != nil {
			return runtime.Result{}, fmt.Errorf("assemble agent request: %w", e)
		}
		response, err = r.Client.Complete(observability.WithStage(ctx, "llm.direct"), prepared.LlmRequest())
	}
	if err != nil {
		return runtime.Result{}, r.incompleteTurn(ctx, inv, completion, fmt.Errorf("complete agent request: %w", err))
	}
	if suppressReply {
		observability.Info(ctx, "agent.response.suppressed", "tool_attempted", meta.ToolAttempted)
		memoryText := strings.TrimSpace(response.Content())
		return runtime.NewToolOwnedResult(inv.RequestID(), memoryText, evidence), nil
	}
	observability.Debug(ctx, "agent.response.finalization_started",
		"finish_reason", response.FinishReason(),
		"tool_call_count", len(response.ToolCalls()),
		"output_chars", len([]rune(response.Content())),
	)
	content, reply, err := finalizeWithContext(r.Finalizer, inv, response.Content(), meta)
	if err != nil {
		observability.Error(ctx, "agent.response.finalization_failed", err)
		return runtime.Result{}, r.incompleteTurn(ctx, inv, completion, fmt.Errorf("finalize agent response: %w", err))
	}
	observability.Info(ctx, "agent.response.finalized", "reply", reply, "output_chars", len([]rune(content)), "evidence_count", len(evidence))
	return runtime.NewResultWithEvidence(inv.RequestID(), content, reply, evidence), nil
}

func (r Runner) incompleteTurn(ctx context.Context, inv runtime.Invocation, completion Completion, cause error) error {
	receipts := completion.Observations.Index(0)
	if len(receipts) == 0 {
		return cause
	}
	failure := &IncompleteTurnError{RequestID: inv.RequestID(), Completion: completion, Cause: cause}
	if r.Memory == nil {
		return failure
	}
	// Durable interruption records contain receipts only. Original argument and
	// result bodies remain in the request-local store, under the original IDs.
	for index := range receipts {
		receipts[index].ArgumentsTruncated = false
	}
	payload, err := json.Marshal(struct {
		RequestID string                        `json:"requestId"`
		Status    string                        `json:"status"`
		Results   []assemble.ObservationReceipt `json:"results"`
	}{RequestID: inv.RequestID(), Status: "interrupted", Results: receipts})
	if err == nil && len(payload) > 32000 {
		err = fmt.Errorf("interrupted execution record exceeds memory budget")
	}
	if err == nil {
		// Cancellation stops tool work, but must not erase an already committed
		// or uncertain effect. Allow one bounded metadata persistence attempt.
		memoryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		err = r.Memory.AppendTurnContext(memoryCtx, apiContext(inv), inv.Prompt(), turn.InterruptedToolTurnPrefix+string(payload), completion.Evidence(), inv.RequestID())
		cancel()
	}
	if err != nil {
		failure.PersistenceError = err
		observability.Error(ctx, "agent.interrupted_execution.persistence_failed", err)
	}
	return failure
}
func (r Runner) loadMemory(ctx context.Context, inv runtime.Invocation) ([]llm.LlmMessage, error) {
	if r.Memory == nil {
		return nil, nil
	}
	return r.Memory.LoadContext(ctx, apiContext(inv), inv.RequestID())
}
func (r Runner) loadHistoricalEvidence(ctx context.Context, inv runtime.Invocation) ([]turn.HistoricalEvidence, error) {
	if r.Memory == nil || inv.Context().Whisper() {
		return nil, nil
	}
	return r.Memory.LoadHistoricalEvidenceContext(ctx, apiContext(inv))
}
func (r Runner) AfterDelivery(ctx context.Context, inv runtime.Invocation, result runtime.Result) error {
	if r.Memory == nil || (!result.ShouldReply() && !result.ToolDeliveryOwned()) {
		return nil
	}
	if err := r.Memory.AppendTurnContext(ctx, apiContext(inv), inv.Prompt(), result.Text(), result.DurableEvidence(), inv.RequestID()); err != nil {
		return fmt.Errorf("agent tool evidence persistence failed: %w", err)
	}
	return nil
}
func apiContext(inv runtime.Invocation) api.Context {
	users := inv.Context().RoomUsers()
	if users == nil {
		users = []string{}
	}
	caps := make([]api.Capability, 0, len(inv.Context().Capabilities()))
	for _, capability := range inv.Context().Capabilities() {
		caps = append(caps, api.Capability(capability))
	}
	ctx, _ := api.NewContextWithCapabilities(inv.Context().Room(), inv.Context().Nick(), inv.Context().Trip(), inv.Context().Hash(), inv.Context().Whisper(), users, caps, inv.Context().ModerationTarget())
	return ctx
}
