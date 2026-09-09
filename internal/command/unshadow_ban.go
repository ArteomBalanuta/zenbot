package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/model"
)

type unshadowBanCommand struct{ commandBase }

func (c *unshadowBanCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 || strings.TrimSpace(a[0]) == "" {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"unban merc")
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.ShadowBans == nil {
		return model.FAILED, fmt.Errorf("shadow-ban persistence unavailable")
	}
	target := strings.TrimSpace(a[0])
	if err := b.ShadowBans.RemoveShadowBan(ctx, target); err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, " unbanned "+target)
	return model.SUCCESSFUL, nil
}
