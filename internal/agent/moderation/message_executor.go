package moderation

import (
	"context"
	"fmt"
)

// AuthoritativeMessageModerator is the fixed privileged engine boundary for
// deterministic message spam actions. It accepts neither user commands nor
// provider/tool payloads.
type AuthoritativeMessageModerator interface {
	WarnFlood(context.Context, string) error
	MutePrincipal(context.Context, string) error
	KickPrincipal(context.Context, string) error
	ShadowBan(context.Context, string) error
}

type MessageActionExecutor interface {
	Execute(context.Context, Decision) error
}

type messageActionExecutor struct{ moderator AuthoritativeMessageModerator }

func NewMessageActionExecutor(moderator AuthoritativeMessageModerator) MessageActionExecutor {
	return &messageActionExecutor{moderator: moderator}
}
func (x *messageActionExecutor) Execute(ctx context.Context, decision Decision) error {
	if x == nil || x.moderator == nil {
		return fmt.Errorf("message moderation executor is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := decision.Validate(); err != nil {
		return err
	}
	switch decision.Action {
	case Warn:
		return x.moderator.WarnFlood(ctx, decision.Principal)
	case Mute:
		return x.moderator.MutePrincipal(ctx, decision.Principal)
	case Kick:
		return x.moderator.KickPrincipal(ctx, decision.Principal)
	case ShadowBan:
		return x.moderator.ShadowBan(ctx, decision.Principal)
	default:
		return fmt.Errorf("action %q is not enabled for message moderation", decision.Action)
	}
}
