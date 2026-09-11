package command

import (
	"context"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/util"
)

type profileCommand struct {
	commandBase
	flair bool
}

func (c *profileCommand) Execute(ctx context.Context) (model.Status, error) {
	kind, example := "color", "00ff00"
	apply := func(operations common.ModerationOperations, target common.NickTarget, value string) error {
		return operations.ForceColor(ctx, target, common.Color(value))
	}
	if c.flair {
		kind, example = "flair", "trusted"
		apply = func(operations common.ModerationOperations, target common.NickTarget, value string) error {
			return operations.ForceFlair(ctx, target, common.Flair(value))
		}
	}
	return executeAppearance(ctx, &c.commandBase, kind, example, apply)
}

type colorCommand struct{ commandBase }
type flairCommand struct{ commandBase }

func (c *colorCommand) Execute(ctx context.Context) (model.Status, error) {
	return (&profileCommand{commandBase: c.commandBase}).Execute(ctx)
}

func (c *flairCommand) Execute(ctx context.Context) (model.Status, error) {
	return (&profileCommand{commandBase: c.commandBase, flair: true}).Execute(ctx)
}

func executeAppearance(ctx context.Context, base *commandBase, kind, example string, apply func(common.ModerationOperations, common.NickTarget, string) error) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(base.message)
	if len(arguments) < 2 {
		if err := replyContext(ctx, base, "\\n Example: "+base.engine.GetPrefix()+kind+" merc "+example); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	target, err := activeModerationTarget(base.engine, arguments[0])
	if err != nil {
		if err := replyContext(ctx, base, base.engine.GetPrefix()+kind+" merc "+example); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	nick, _ := util.NormalizeNickTarget(&arguments[0])
	if target == nil {
		if err := replyContext(ctx, base, "User "+nick+" is not in the room, "+kind+" was not applied."); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	operations, err := moderationOperations(base.engine)
	if err != nil {
		return model.FAILED, err
	}
	if err := apply(operations, common.NickTarget(target.Name), arguments[1]); err != nil {
		return model.FAILED, err
	}
	if kind == "flair" {
		if err := replyContext(ctx, base, "\\n Flair request sent; server application is unconfirmed."); err != nil {
			return model.FAILED, err
		}
	}
	return model.SUCCESSFUL, nil
}

var _ common.SaturnCommand = (*colorCommand)(nil)
var _ common.SaturnCommand = (*flairCommand)(nil)
