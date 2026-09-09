package command

import (
	"context"

	"zenbot/internal/model"
)

type unshadowBanCommand struct{ commandBase }

func (c *unshadowBanCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) == 0 {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"unban merc")
		return model.FAILED, nil
	}
	service, err := shadowBanService(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if hasArgument(arguments, "-all") {
		records, err := service.List(ctx)
		if err != nil {
			return model.FAILED, err
		}
		if len(records) == 0 {
			reply(&c.commandBase, "No users has been banned.")
			return model.FAILED, nil
		}
		if err := service.RemoveAll(ctx); err != nil {
			return model.FAILED, err
		}
		if err := ctx.Err(); err != nil {
			return model.FAILED, err
		}
		reply(&c.commandBase, "Unbanned hashes, trips, nicks: \\n"+formatShadowBanRecords(records))
		return model.FAILED, nil
	}
	target := arguments[0]
	if err := service.Remove(ctx, target); err != nil {
		return model.FAILED, err
	}
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, " unbanned "+target)
	return model.SUCCESSFUL, nil
}

func hasArgument(arguments []string, value string) bool {
	for _, argument := range arguments {
		if argument == value {
			return true
		}
	}
	return false
}
