package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type prefixCommand struct{ commandBase }

func (c *prefixCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) == 0 || strings.TrimSpace(arguments[0]) == "" {
		if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"prefix $"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	controller, ok := c.engine.(common.PrefixController)
	if !ok {
		return model.FAILED, fmt.Errorf("prefix controller is unavailable")
	}
	prefix := strings.TrimSpace(arguments[0])
	previous, err := controller.UpdatePrefix(prefix)
	if err != nil {
		return model.FAILED, err
	}
	if err := replyContext(ctx, &c.commandBase, "prefix changed from "+previous+" to "+prefix); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
