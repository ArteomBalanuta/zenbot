package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/model"
)

type lastonlineCommand struct{ commandBase }

func (c *lastonlineCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) == 0 {
		reply(&c.commandBase, "\\n Example: "+c.engine.GetPrefix()+"lastseen merc")
		return model.FAILED, nil
	}
	target := strings.TrimSpace(arguments[0])
	if strings.HasPrefix(target, "@") {
		target = strings.TrimSpace(strings.TrimPrefix(target, "@"))
	}
	if target == "" {
		reply(&c.commandBase, "\\n Example: "+c.engine.GetPrefix()+"lastseen merc")
		return model.FAILED, nil
	}
	services := bundle(c.engine)
	if services == nil || services.Users == nil || services.Users.Queries == nil {
		return model.FAILED, fmt.Errorf("last-online user queries unavailable")
	}
	text, err := services.Users.LastOnline(ctx, target)
	if err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, text)
	return model.SUCCESSFUL, nil
}
