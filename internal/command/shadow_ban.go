package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/model"
)

type shadowBanCommand struct{ commandBase }

func (c *shadowBanCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 || strings.TrimSpace(a[0]) == "" {
		reply(&c.commandBase, "Example:"+c.engine.GetPrefix()+"shadowban merc")
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.ShadowBans == nil {
		return model.FAILED, fmt.Errorf("shadow-ban persistence unavailable")
	}
	target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(a[0]), "@"))
	if err := b.ShadowBans.PersistShadowBanSelector(ctx, target, ""); err != nil {
		return model.FAILED, err
	}
	if u := c.engine.GetActiveUserByName(target); u != nil {
		c.engine.Kick(u.Name, "abcdef")
		reply(&c.commandBase, "shadow_banned: "+target+" trip: "+u.Trip+" hash: "+u.Hash)
	} else {
		reply(&c.commandBase, "banned: "+target)
	}
	return model.SUCCESSFUL, nil
}
