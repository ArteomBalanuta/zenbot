package command

import (
	"context"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type unmuteCommand struct{ commandBase }

func (c *unmuteCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) == 0 {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"unmute jJ4M4fsECSazzlj")
		return model.FAILED, nil
	}
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	hash := arguments[0]
	if err := operations.UnmuteHash(ctx, common.BanHash(hash)); err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, hash+" has been unmuted")
	return model.SUCCESSFUL, nil
}
