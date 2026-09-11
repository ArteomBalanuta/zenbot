package command

import (
	"context"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type overflowCommand struct{ commandBase }

func (c *overflowCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) != 1 {
		if err := replyContext(ctx, &c.commandBase, "target nick isn't set! Example: "+c.engine.GetPrefix()+"shoot @merc"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	target, err := activeModerationTarget(c.engine, arguments[0])
	if err != nil {
		return model.FAILED, err
	}
	if target == nil {
		return model.FAILED, nil
	}
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if err = operations.OverflowNick(ctx, common.NickTarget(target.Name)); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
