package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/model"
)

type lastOnlineCommand struct{ commandBase }

func (c *lastOnlineCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) == 0 {
		reply(&c.commandBase, "\\n Example: "+c.engine.GetPrefix()+"lastseen merc")
		return model.FAILED, nil
	}
	target := strings.TrimSpace(arguments[0])
	target = strings.TrimSpace(strings.TrimPrefix(target, "@"))
	if target == "" {
		reply(&c.commandBase, "\\n Example: "+c.engine.GetPrefix()+"lastseen merc")
		return model.FAILED, nil
	}
	s := userService(c.engine)
	if s == nil {
		return model.FAILED, fmt.Errorf("user service unavailable")
	}
	text, err := s.LastOnline(ctx, target)
	if err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, text)
	return model.SUCCESSFUL, nil
}
