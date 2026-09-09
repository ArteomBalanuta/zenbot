package command

import (
	"context"
	"fmt"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/util"
)

func moderationOperations(engine common.Engine) (common.ModerationOperations, error) {
	operations, ok := engine.(common.ModerationOperations)
	if !ok {
		return nil, fmt.Errorf("moderation operations unavailable")
	}
	return operations, nil
}

// activeModerationTarget resolves source commands against the current room
// snapshot, preserving the active user's canonical nick and hash.
func activeModerationTarget(engine common.Engine, raw string) (*model.User, error) {
	nick, err := util.NormalizeNickTarget(&raw)
	if err != nil {
		return nil, err
	}
	if user := engine.GetActiveUserByName(nick); user != nil {
		return user, nil
	}
	return nil, nil
}

type muteCommand struct{ commandBase }

func (c *muteCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) == 0 {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"mute merc")
		return model.FAILED, nil
	}
	target, err := activeModerationTarget(c.engine, arguments[0])
	if err != nil {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"mute merc")
		return model.FAILED, nil
	}
	nick, _ := util.NormalizeNickTarget(&arguments[0])
	if target == nil {
		reply(&c.commandBase, nick+" is not in the room")
		return model.FAILED, nil
	}
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if err := operations.MuteNick(ctx, common.NickTarget(target.Name)); err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, target.Name+" "+target.Hash+" has been muted")
	return model.SUCCESSFUL, nil
}

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

type colorCommand struct{ commandBase }
type flairCommand struct{ commandBase }

func (c *colorCommand) Execute(ctx context.Context) (model.Status, error) {
	return executeAppearance(ctx, &c.commandBase, "color", "00ff00", func(operations common.ModerationOperations, target common.NickTarget, value string) error {
		return operations.ForceColor(ctx, target, common.Color(value))
	})
}

func (c *flairCommand) Execute(ctx context.Context) (model.Status, error) {
	return executeAppearance(ctx, &c.commandBase, "flair", "trusted", func(operations common.ModerationOperations, target common.NickTarget, value string) error {
		return operations.ForceFlair(ctx, target, common.Flair(value))
	})
}

func executeAppearance(ctx context.Context, base *commandBase, kind, example string, apply func(common.ModerationOperations, common.NickTarget, string) error) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(base.message)
	if len(arguments) < 2 {
		reply(base, "\\n Example: "+base.engine.GetPrefix()+kind+" merc "+example)
		return model.FAILED, nil
	}
	target, err := activeModerationTarget(base.engine, arguments[0])
	if err != nil {
		reply(base, base.engine.GetPrefix()+kind+" merc "+example)
		return model.FAILED, nil
	}
	nick, _ := util.NormalizeNickTarget(&arguments[0])
	if target == nil {
		reply(base, "User "+nick+" is not in the room, "+kind+" was not applied.")
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
		reply(base, "\\n Flair set successfully!")
	}
	return model.SUCCESSFUL, nil
}

var _ common.SaturnCommand = (*muteCommand)(nil)
var _ common.SaturnCommand = (*unmuteCommand)(nil)
var _ common.SaturnCommand = (*colorCommand)(nil)
var _ common.SaturnCommand = (*flairCommand)(nil)
