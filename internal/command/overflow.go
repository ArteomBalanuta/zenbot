package command

import (
	"context"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/util"
)

type overflowCommand struct{ commandBase }

func (c *overflowCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	target, ok := firstModerationArgument(c.message)
	if !ok {
		if err := replyContext(ctx, &c.commandBase, "target nick isn't set! Example: "+c.engine.GetPrefix()+"shoot @merc"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	normalized, err := util.NormalizeNickTarget(&target)
	if err != nil {
		return model.FAILED, err
	}
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if err = operations.OverflowNick(ctx, common.NickTarget(normalized)); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
