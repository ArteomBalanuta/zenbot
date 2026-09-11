package command

import (
	"context"
	"strings"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/util"
)

func firstModerationArgument(message *model.ChatMessage) (string, bool) {
	arguments := args(message)
	if len(arguments) == 0 {
		return "", false
	}
	return arguments[0], true
}

type moderationIdentityCommand struct {
	commandBase
	deauthorize bool
}

func (c *moderationIdentityCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	trip, ok := firstModerationArgument(c.message)
	if !ok {
		name := "auth"
		if c.deauthorize {
			name = "deauth"
		}
		if err := replyContext(ctx, &c.commandBase, " example: "+c.engine.GetPrefix()+name+" cmdTV+"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if c.deauthorize {
		err = operations.DeauthorizeTrip(ctx, common.Trip(trip))
	} else {
		err = operations.AuthorizeTrip(ctx, common.Trip(trip))
	}
	if err != nil {
		return model.FAILED, err
	}
	verb := " authorized"
	if c.deauthorize {
		verb = " deauthorized"
	}
	if err := replyContext(ctx, &c.commandBase, verb+" trip: "+trip); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type simpleBanCommand struct{ commandBase }

func (c *simpleBanCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	target, ok := firstModerationArgument(c.message)
	if !ok {
		if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"ban merc"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	normalized, err := util.NormalizeNickTarget(&target)
	if err != nil {
		if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"ban merc"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if err = operations.BanNick(ctx, common.NickTarget(normalized)); err != nil {
		return model.FAILED, err
	}
	if err := replyContext(ctx, &c.commandBase, normalized+" has been banned"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type simpleUnbanCommand struct{ commandBase }

func (c *simpleUnbanCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	hash, ok := firstModerationArgument(c.message)
	if !ok || strings.TrimSpace(hash) == "" {
		if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"unban HjkUEWNlIRH35Xk"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if err = operations.UnbanHash(ctx, common.BanHash(hash)); err != nil {
		return model.FAILED, err
	}
	if err := replyContext(ctx, &c.commandBase, hash+" has been unbanned"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type simpleUnbanAllCommand struct{ commandBase }

func (c *simpleUnbanAllCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if err = operations.UnbanAllContext(ctx); err != nil {
		return model.FAILED, err
	}
	if err := replyContext(ctx, &c.commandBase, "mercy."); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type simpleLockCommand struct{ commandBase }

func (c *simpleLockCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	argument, ok := firstModerationArgument(c.message)
	if !ok || (argument != "on" && argument != "off") {
		if err := replyContext(ctx, &c.commandBase, c.engine.GetPrefix()+"lock [on|off]"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if argument == "on" {
		err = operations.LockRoom(ctx)
		if err == nil {
			if err := replyContext(ctx, &c.commandBase, " Room locked!"); err != nil {
				return model.FAILED, err
			}
		}
	} else {
		err = operations.UnlockRoom(ctx)
		if err == nil {
			if err := replyContext(ctx, &c.commandBase, " Room unlocked!"); err != nil {
				return model.FAILED, err
			}
		}
	}
	if err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
