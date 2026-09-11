package command

import (
	"context"
	"errors"
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
		if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"remove [merc|g0KY09]"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, fmt.Errorf("identity selector is required")
	}
	s := userService(c.engine)
	if s == nil {
		return model.FAILED, fmt.Errorf("user service unavailable")
	}
	if _, err := s.DeleteIdentity(ctx, strings.TrimSpace(a[0])); err != nil {
		if sendErr := replyContext(ctx, &c.commandBase, "Something went wrong deleting the user"); sendErr != nil {
			return model.FAILED, errors.Join(err, sendErr)
		}
		return model.FAILED, err
	}
	if err := replyContext(ctx, &c.commandBase, "User has been removed successfully"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
