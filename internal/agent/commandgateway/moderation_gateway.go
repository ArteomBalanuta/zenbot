package commandgateway

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"zenbot/internal/agent/api"
	"zenbot/internal/core"
)

// ModerationAction is the closed autonomous moderation action inventory.
type ModerationAction string

const (
	ActionCaptcha     ModerationAction = "captcha"
	ActionMute        ModerationAction = "mute"
	ActionUnmute      ModerationAction = "unmute"
	ActionKick        ModerationAction = "kick"
	ActionShadowBan   ModerationAction = "shadowban"
	ActionUnshadowBan ModerationAction = "unshadowban"
)

func ParseModerationAction(value string) (ModerationAction, error) {
	action := ModerationAction(value)
	switch action {
	case ActionCaptcha, ActionMute, ActionUnmute, ActionKick, ActionShadowBan, ActionUnshadowBan:
		return action, nil
	default:
		return "", fmt.Errorf("unsupported moderation action")
	}
}

// ModerationOperator is intentionally narrower than public command execution.
type ModerationOperator interface {
	EnableCaptcha(context.Context) error
	MutePrincipal(context.Context, string) error
	UnmuteTarget(context.Context, core.ModerationTarget) error
	KickPrincipal(context.Context, string) error
	ShadowBan(context.Context, string) error
	UnshadowBanTarget(context.Context, core.ModerationTarget) error
}

type ModerationGateway struct{ operator ModerationOperator }

func NewModerationGateway(operator ModerationOperator) *ModerationGateway {
	return &ModerationGateway{operator: operator}
}

func (g *ModerationGateway) Execute(ctx context.Context, mode api.InvocationMode, caller api.Context, target core.ModerationTarget, action ModerationAction) error {
	if ctx == nil {
		return errors.New("moderation context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if g == nil || g.operator == nil {
		return errors.New("moderation operator is unavailable")
	}
	if mode != api.MODERATION || caller.Whisper() || strings.TrimSpace(caller.Nick()) == "" || !caller.HasCapability(api.ModerationCommands) {
		return errors.New("moderation invocation is unauthorized")
	}
	if _, err := ParseModerationAction(string(action)); err != nil {
		return err
	}
	if action != ActionCaptcha && strings.TrimSpace(target.Name) == "" {
		return errors.New("moderation target name is blank")
	}
	switch action {
	case ActionCaptcha:
		return g.operator.EnableCaptcha(ctx)
	case ActionMute:
		return g.operator.MutePrincipal(ctx, target.Name)
	case ActionUnmute:
		if strings.TrimSpace(target.Hash) == "" {
			return errors.New("moderation target hash is blank")
		}
		return g.operator.UnmuteTarget(ctx, target)
	case ActionKick:
		return g.operator.KickPrincipal(ctx, target.Name)
	case ActionShadowBan:
		return g.operator.ShadowBan(ctx, target.Name)
	case ActionUnshadowBan:
		return g.operator.UnshadowBanTarget(ctx, target)
	default:
		return errors.New("unsupported moderation action")
	}
}
