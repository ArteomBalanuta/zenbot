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

type captchaCommand struct{ commandBase }

func (c *captchaCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	argument, present := firstModerationArgument(c.message)
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	switch {
	case !present || argument == "on":
		err = operations.EnableCaptcha(ctx)
		if err == nil {
			reply(&c.commandBase, " Captcha enabled!")
		}
	case argument == "off":
		err = operations.DisableCaptcha(ctx)
		if err == nil {
			reply(&c.commandBase, " Captcha disabled!")
		}
	default:
		reply(&c.commandBase, c.engine.GetPrefix()+"captcha [on|off]")
		return model.FAILED, nil
	}
	if err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
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
		reply(&c.commandBase, " example: "+c.engine.GetPrefix()+name+" cmdTV+")
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
	reply(&c.commandBase, verb+" trip: "+trip)
	return model.SUCCESSFUL, nil
}

type overflowCommand struct{ commandBase }

func (c *overflowCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	target, ok := firstModerationArgument(c.message)
	if !ok {
		reply(&c.commandBase, "target nick isn't set! Example: "+c.engine.GetPrefix()+"shoot @merc")
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

type simpleBanCommand struct{ commandBase }

func (c *simpleBanCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	target, ok := firstModerationArgument(c.message)
	if !ok {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"ban merc")
		return model.FAILED, nil
	}
	normalized, err := util.NormalizeNickTarget(&target)
	if err != nil {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"ban merc")
		return model.FAILED, nil
	}
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if err = operations.BanNick(ctx, common.NickTarget(normalized)); err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, normalized+" has been banned")
	return model.SUCCESSFUL, nil
}

type simpleUnbanCommand struct{ commandBase }

func (c *simpleUnbanCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	hash, ok := firstModerationArgument(c.message)
	if !ok || strings.TrimSpace(hash) == "" {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"unban HjkUEWNlIRH35Xk")
		return model.FAILED, nil
	}
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if err = operations.UnbanHash(ctx, common.BanHash(hash)); err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, hash+" has been unbanned")
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
	reply(&c.commandBase, "mercy.")
	return model.SUCCESSFUL, nil
}

type simpleLockCommand struct{ commandBase }

func (c *simpleLockCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	argument, ok := firstModerationArgument(c.message)
	if !ok || (argument != "on" && argument != "off") {
		reply(&c.commandBase, c.engine.GetPrefix()+"lock [on|off]")
		return model.FAILED, nil
	}
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if argument == "on" {
		err = operations.LockRoom(ctx)
		if err == nil {
			reply(&c.commandBase, " Room locked!")
		}
	} else {
		err = operations.UnlockRoom(ctx)
		if err == nil {
			reply(&c.commandBase, " Room unlocked!")
		}
	}
	if err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
