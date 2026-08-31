package moderation

import (
	"context"
	"fmt"
)

// AuthoritativeEngine is the small privileged boundary supplied at composition.
// It deliberately accepts no caller, model, command, or tool inputs.
type AuthoritativeEngine interface {
	EnableCaptcha(context.Context) error
	ShadowBan(context.Context, string) error
}
type EngineActionExecutor struct{ engine AuthoritativeEngine }

func NewEngineActionExecutor(engine AuthoritativeEngine) *EngineActionExecutor {
	return &EngineActionExecutor{engine: engine}
}
func (x *EngineActionExecutor) Execute(ctx context.Context, d Decision) error {
	if x == nil || x.engine == nil {
		return fmt.Errorf("moderation executor is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := d.Validate(); err != nil {
		return err
	}
	switch d.Action {
	case Captcha:
		return x.engine.EnableCaptcha(ctx)
	case ShadowBan:
		return x.engine.ShadowBan(ctx, d.Principal)
	default:
		return fmt.Errorf("action %q is not enabled for join moderation", d.Action)
	}
}
