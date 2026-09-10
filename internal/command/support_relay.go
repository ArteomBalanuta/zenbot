package command

import (
	"context"
	"fmt"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type supportRelayCommand struct {
	commandBase
	anonymous bool
}

func (c *supportRelayCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	relay, ok := c.engine.(common.SupportReplicaRelay)
	if !ok {
		return model.FAILED, fmt.Errorf("support replica relay is not configured")
	}
	if err := relay.RelayToSupport(ctx, common.SupportRelayRequest{
		Author: c.message.Name, Trip: c.message.Trip, Arguments: args(c.message), Anonymous: c.anonymous,
	}); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func (c *supportRelayCommand) NewInstance(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	return &supportRelayCommand{commandBase: commandBase{engine: e, message: m, role: c.role, aliases: c.aliases, canonical: c.canonical}, anonymous: c.anonymous}
}
