package live

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/participation"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/agent/turn"
	"zenbot/internal/model"
)

type DirectInvoker struct {
	Assembler           *assemble.Assembler
	Client              llm.LlmClient
	ConversationContext ConversationContextProvider
	ToolLoop            *ToolLoop
	Finalizer           Finalizer
	Memory              *turn.TurnMemory
}

func (i DirectInvoker) Invoke(ctx context.Context, message *model.ChatMessage, prompt string) (string, error) {
	completion, err := i.InvokeCompletion(ctx, message, prompt)
	return completion.Text(), err
}

// InvokeCompletion returns the direct response and request-local evidence without retaining mutable state.
func (i DirectInvoker) InvokeCompletion(ctx context.Context, message *model.ChatMessage, prompt string) (runtime.DirectCompletion, error) {
	if i.Assembler == nil || i.Client == nil {
		return runtime.DirectCompletion{}, fmt.Errorf("direct agent invoker is not initialized")
	}
	if message == nil {
		return runtime.DirectCompletion{}, fmt.Errorf("direct agent message is missing")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return runtime.DirectCompletion{}, fmt.Errorf("l requires a prompt")
	}
	inv := runtime.NewInvocation(requestID(), runtime.NewContext(message.Channel, message.Name, message.Trip, message.Hash, message.Whisper || message.IsWhisper, nil), prompt, runtime.DIRECT, message.Text, true)
	var memory []llm.LlmMessage
	var historical []turn.HistoricalEvidence
	var err error
	if i.Memory != nil {
		memory, err = i.Memory.LoadContext(ctx, apiContext(inv), inv.RequestID())
		if err != nil {
			return runtime.DirectCompletion{}, err
		}
		if !inv.Context().Whisper() {
			historical, err = i.Memory.LoadHistoricalEvidenceContext(ctx, apiContext(inv))
			if err != nil {
				return runtime.DirectCompletion{}, err
			}
		}
	}
	recent, err := loadRecentContext(ctx, i.ConversationContext, inv)
	if err != nil {
		return runtime.DirectCompletion{}, err
	}
	var response llm.LlmResponse
	var evidence []turn.PersistableEvidence
	suppressReply := false
	var meta FinalizationContext
	if i.ToolLoop != nil {
		completion, loopErr := i.ToolLoop.CompleteWithEvidenceAndHistorical(ctx, inv, memory, recent, historical)
		response, err, evidence, suppressReply = completion.Response, loopErr, completion.Evidence(), completion.SuppressReply
		meta = FinalizationContext{CandidateKind: completion.CandidateKind, ToolAttempted: completion.ToolAttempted}
	} else {
		meta.CandidateKind = participation.Classifier{}.Classify(inv.Prompt())
		prepared, e := i.Assembler.AssembleWithHistoricalEvidence(ctx, inv, memory, recent, nil, assemble.Talk, historical)
		if e != nil {
			return runtime.DirectCompletion{}, e
		}
		if !inv.Context().Whisper() && prepared.RequiredFreshTool() != "" {
			return runtime.DirectCompletion{}, fmt.Errorf("required fresh history needs bounded tool loop")
		}
		response, err = i.Client.Complete(ctx, prepared.LlmRequest())
	}
	if err != nil {
		return runtime.DirectCompletion{}, err
	}
	if suppressReply {
		return runtime.DirectCompletion{}, nil
	}
	if i.Finalizer != nil {
		text, reply, e := finalizeWithContext(i.Finalizer, inv, response.Content(), meta)
		if e != nil {
			return runtime.DirectCompletion{}, e
		}
		if !reply {
			return runtime.DirectCompletion{}, nil
		}
		return runtime.NewDirectCompletion(text, evidence), nil
	}
	if text := strings.TrimSpace(response.Content()); text != "" {
		return runtime.NewDirectCompletion(text, evidence), nil
	}
	return runtime.DirectCompletion{}, fmt.Errorf("agent returned an empty response")
}
func (i DirectInvoker) Persist(ctx context.Context, message *model.ChatMessage, prompt, text string) error {
	return i.PersistDelivery(ctx, message, prompt, runtime.NewDirectCompletion(text, nil))
}

// PersistDelivery runs only after the visible direct send and consumes its immutable artifact.
func (i DirectInvoker) PersistDelivery(ctx context.Context, message *model.ChatMessage, prompt string, completion runtime.DirectCompletion) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	text := completion.Text()
	if i.Memory == nil || strings.TrimSpace(text) == "" {
		return nil
	}
	if message == nil {
		return fmt.Errorf("direct agent message is missing")
	}
	inv := runtime.NewInvocation("direct-persist", runtime.NewContext(message.Channel, message.Name, message.Trip, message.Hash, message.Whisper || message.IsWhisper, nil), prompt, runtime.DIRECT, message.Text, true)
	if err := i.Memory.AppendContext(ctx, apiContext(inv), prompt, text, inv.RequestID()); err != nil {
		return err
	}
	if inv.Context().Whisper() || len(completion.DurableEvidence()) == 0 {
		return nil
	}
	if err := i.Memory.AppendToolEvidenceContext(ctx, apiContext(inv), completion.DurableEvidence()); err != nil {
		return fmt.Errorf("agent tool evidence persistence failed: %w", err)
	}
	return nil
}
func requestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "direct-agent-request"
	}
	return fmt.Sprintf("%x", bytes)
}
