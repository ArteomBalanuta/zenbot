package live

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/participation"
	"zenbot/internal/model"
)

// AgentService is the application-private live-agent submission boundary.
type AgentService interface {
	Submit(api.Invocation) error
	Close()
}

// invocationFactory permits the adapter's invalid-input contract to verify that
// no invocation is created before validation succeeds.
type invocationFactory interface {
	Create(participation.TrustedSnapshot, model.ChatMessage, string, api.InvocationMode, bool) (api.Invocation, error)
}

// DirectSubmissionAdapter converts trusted listener state into one DIRECT invocation.
type DirectSubmissionAdapter struct {
	Service  AgentService
	Factory  invocationFactory
	Snapshot func() participation.TrustedSnapshot
}

func (a DirectSubmissionAdapter) Submit(ctx context.Context, message *model.ChatMessage, prompt string) error {
	return a.submit(ctx, message, prompt, api.DIRECT)
}

func (a DirectSubmissionAdapter) SubmitVibe(ctx context.Context, message *model.ChatMessage) error {
	return a.submit(ctx, message, "Give a short vibe check of the recent room conversation and its active participants.", api.VIBE)
}

func (a DirectSubmissionAdapter) submit(ctx context.Context, message *model.ChatMessage, prompt string, mode api.InvocationMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if message == nil || a.Service == nil {
		return fmt.Errorf("direct agent submission is unavailable")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return fmt.Errorf("direct agent prompt is required")
	}
	snapshot := a.Snapshot()
	if strings.TrimSpace(snapshot.Room) == "" {
		return fmt.Errorf("direct agent room is required")
	}
	snapshot.Users = append([]string(nil), snapshot.Users...)
	invocation, err := a.Factory.Create(snapshot, *message, prompt, mode, true)
	if err != nil {
		return err
	}
	return a.Service.Submit(invocation)
}
