package core

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/repository"
)

// ModerationReverser performs the two source-compatible typed moderation
// reversal operations against a listener-resolved ModerationTarget.
type ModerationReverser interface {
	UnmuteTarget(context.Context, ModerationTarget) error
	UnshadowBanTarget(context.Context, ModerationTarget) error
}

// UnmuteTarget sends Saturn's exact unmute wire operation with the captured raw
// listener-resolved hash. It deliberately accepts no free-form target string.
func (e *EngineImpl) UnmuteTarget(ctx context.Context, target ModerationTarget) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(target.Hash) == "" {
		return fmt.Errorf("moderation target hash is blank")
	}
	return e.sendModerationCommand(ctx, "unmute", map[string]string{"hash": target.Hash})
}

// UnshadowBanTarget removes source-compatible shadow-ban records for the
// captured authoritative name. It sends no chat-server protocol payload.
func (e *EngineImpl) UnshadowBanTarget(ctx context.Context, target ModerationTarget) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(target.Name) == "" {
		return fmt.Errorf("moderation target name is blank")
	}
	repo, ok := e.Repository.(repository.ShadowBanReversalRepository)
	if !ok {
		return fmt.Errorf("authoritative shadow-ban reversal repository is unavailable")
	}
	return repo.RemoveShadowBanBySourceTarget(ctx, target.Name)
}
