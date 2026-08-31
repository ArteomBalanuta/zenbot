package moderation

import (
	"context"
	"fmt"
	"strings"
)

type Action string

const (
	Warn      Action = "warning"
	Captcha   Action = "captcha"
	Mute      Action = "mute"
	Kick      Action = "kick"
	ShadowBan Action = "shadow-ban"
)

type Decision struct {
	Action            Action
	Principal, Reason string
}

// Validate enforces the autonomous action/target boundary before dispatch.
func (d Decision) Validate() error {
	switch d.Action {
	case Captcha:
		if strings.TrimSpace(d.Principal) != "" {
			return fmt.Errorf("captcha decision must not target a principal")
		}
	case Warn, Mute, Kick, ShadowBan:
		principal := strings.TrimSpace(d.Principal)
		if principal == "" {
			return fmt.Errorf("%s decision requires a principal", d.Action)
		}
		if d.Principal != principal {
			return fmt.Errorf("%s decision principal must be normalized", d.Action)
		}
	default:
		return fmt.Errorf("unknown moderation action %q", d.Action)
	}
	return nil
}

type ActionExecutor interface {
	Execute(context.Context, Decision) error
}
