package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/model"
)

type removeCommand struct{ commandBase }

func (c *removeCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 || strings.TrimSpace(a[0]) == "" {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"remove [merc|g0KY09]")
		return model.FAILED, fmt.Errorf("identity selector is required")
	}
	s := userService(c.engine)
	if s == nil {
		return model.FAILED, fmt.Errorf("user service unavailable")
	}
	if _, err := s.DeleteIdentity(ctx, strings.TrimSpace(a[0])); err != nil {
		reply(&c.commandBase, "Something went wrong deleting the user")
		return model.FAILED, err
	}
	reply(&c.commandBase, "User has been removed successfully")
	return model.SUCCESSFUL, nil
}
