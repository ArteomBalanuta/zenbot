package command

import (
	"context"
	"fmt"
	"strings"

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
	users := engine.GetActiveUsers()
	if users == nil {
		return nil, nil
	}
	for user := range *users {
		if user != nil && strings.EqualFold(user.Name, nick) {
			return user, nil
		}
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
		if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"mute merc"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	target, err := activeModerationTarget(c.engine, arguments[0])
	if err != nil {
		if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"mute merc"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	nick, _ := util.NormalizeNickTarget(&arguments[0])
	if target == nil {
		if err := replyContext(ctx, &c.commandBase, nick+" is not in the room"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if err := operations.MuteNick(ctx, common.NickTarget(target.Name)); err != nil {
		return model.FAILED, err
	}
	if err := replyContext(ctx, &c.commandBase, target.Name+" "+target.Hash+" has been muted"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

var _ common.SaturnCommand = (*muteCommand)(nil)
