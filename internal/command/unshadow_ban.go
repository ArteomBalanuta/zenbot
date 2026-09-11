package command

import (
	"context"
	"fmt"

	"zenbot/internal/model"
)

type unshadowBanCommand struct{ commandBase }

func (c *unshadowBanCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) != 1 {
		if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"unban merc"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	service, err := shadowBanService(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if arguments[0] == "-all" {
		changed, err := service.RemoveAll(ctx)
		if err != nil {
			return model.FAILED, err
		}
		if changed == 0 {
			if err := replyContext(ctx, &c.commandBase, "No shadow-ban records matched."); err != nil {
				return model.FAILED, err
			}
			return model.FAILED, nil
		}
		if err := ctx.Err(); err != nil {
			return model.FAILED, err
		}
		if err := replyContext(ctx, &c.commandBase, fmt.Sprintf("Unbanned shadow-ban records: %d", changed)); err != nil {
			return model.FAILED, err
		}
		return model.SUCCESSFUL, nil
	}
	target := arguments[0]
	changed, err := service.Remove(ctx, target)
	if err != nil {
		return model.FAILED, err
	}
	if changed == 0 {
		if err := replyContext(ctx, &c.commandBase, "No shadow-ban records matched "+target+"."); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if err := replyContext(ctx, &c.commandBase, fmt.Sprintf("Unbanned shadow-ban records: %d", changed)); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
