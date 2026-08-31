package command

import (
	"context"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type subscribeCommand struct{ commandBase }

func (c *subscribeCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if c.message.Trip == "" {
		reply(&c.commandBase, "you have to set your trip to use this command.")
		return model.FAILED, nil
	}
	c.engine.SubscribeTrip(c.message.Trip)
	reply(&c.commandBase, "your trip will be whispered hashes and nicks for each new joining user. ")
	return model.SUCCESSFUL, nil
}

type unsubscribeCommand struct{ commandBase }

func (c *unsubscribeCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if c.message.Trip == "" || !c.engine.IsSubscribedTrip(c.message.Trip) {
		reply(&c.commandBase, "you are not subscribed, please set your trip and use "+c.engine.GetPrefix()+" sub command.")
		return model.FAILED, nil
	}
	c.engine.UnsubscribeTrip(c.message.Trip)
	reply(&c.commandBase, "your trip will no longer receive hashes and nicks for each new joining user. ")
	return model.SUCCESSFUL, nil
}

func newSubscriptionCommand(canonical string, aliases []string, role model.Role, e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	base := commandBase{engine: e, message: m, role: role, aliases: aliases}
	if canonical == "sub" {
		return &subscribeCommand{base}
	}
	return &unsubscribeCommand{base}
}
