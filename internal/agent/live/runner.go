package live

import (
	"context"
	"fmt"
	"strings"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/llm"
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
	Catalog        *verifiedQuoteCatalog
}

func NewOutputFinalizer(noReplyMarker string, maxOutputChars int) (OutputFinalizer, error) {
	catalog, err := loadVerifiedQuoteCatalog(nil)
	if err != nil {
		return OutputFinalizer{}, err
	}
	return OutputFinalizer{NoReplyMarker: noReplyMarker, MaxOutputChars: maxOutputChars, Catalog: &catalog}, nil
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
	if quoteOnlyRequired(inv, meta) {
		if f.Catalog == nil {
			return "", false, fmt.Errorf("verified quote catalog is not initialized")
		}
		content = f.Catalog.selectVerifiedOrFallback(content)
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

func quoteOnlyRequired(inv runtime.Invocation, meta FinalizationContext) bool {
	return !inv.Context().Whisper() && !inv.CommandOriginated() && inv.Mode() != runtime.MODERATION && !meta.ToolAttempted && (meta.CandidateKind == participation.Talk || meta.CandidateKind == participation.Unclassified)
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
		return runtime.Result{}, err
	}
	historical, err := r.loadHistoricalEvidence(ctx, inv)
	if err != nil {
		return runtime.Result{}, err
	}
	recent, err := loadRecentContext(ctx, r.ConversationContext, inv)
	if err != nil {
		return runtime.Result{}, err
	}
	var response llm.LlmResponse
	var evidence []turn.PersistableEvidence
	suppressReply := false
	var meta FinalizationContext
	if r.ToolLoop != nil {
		completion, loopErr := r.ToolLoop.CompleteWithEvidenceAndHistorical(ctx, inv, memory, recent, historical)
		response, err, evidence, suppressReply = completion.Response, loopErr, completion.Evidence(), completion.SuppressReply
		meta = FinalizationContext{CandidateKind: completion.CandidateKind, ToolAttempted: completion.ToolAttempted}
	} else {
		meta.CandidateKind = participation.Classifier{}.Classify(inv.Prompt())
		prepared, e := r.Assembler.AssembleWithHistoricalEvidence(ctx, inv, memory, recent, nil, assemble.Talk, historical)
		if e != nil {
			return runtime.Result{}, fmt.Errorf("assemble agent request: %w", e)
		}
		if !inv.Context().Whisper() && prepared.RequiredFreshTool() != "" {
			return runtime.Result{}, fmt.Errorf("required fresh history needs bounded tool loop")
		}
		response, err = r.Client.Complete(ctx, prepared.LlmRequest())
	}
	if err != nil {
		return runtime.Result{}, fmt.Errorf("complete agent request: %w", err)
	}
	if suppressReply {
		return runtime.NewResultWithEvidence(inv.RequestID(), "", false, nil), nil
	}
	content, reply, err := finalizeWithContext(r.Finalizer, inv, response.Content(), meta)
	if err != nil {
		return runtime.Result{}, fmt.Errorf("finalize agent response: %w", err)
	}
	return runtime.NewResultWithEvidence(inv.RequestID(), content, reply, evidence), nil
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
	if r.Memory == nil || !result.ShouldReply() {
		return nil
	}
	if err := r.Memory.AppendContext(ctx, apiContext(inv), inv.Prompt(), result.Text(), inv.RequestID()); err != nil {
		return err
	}
	if inv.Context().Whisper() || len(result.DurableEvidence()) == 0 {
		return nil
	}
	if err := r.Memory.AppendToolEvidenceContext(ctx, apiContext(inv), result.DurableEvidence()); err != nil {
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
