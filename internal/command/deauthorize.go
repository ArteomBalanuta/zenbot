package command

import (
	"context"

	"zenbot/internal/model"
)

type deauthorizeCommand struct{ commandBase }

func (c *deauthorizeCommand) Execute(ctx context.Context) (model.Status, error) {
	return (&moderationIdentityCommand{commandBase: c.commandBase, deauthorize: true}).Execute(ctx)
}
