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
	if len(arguments) == 0 {
		if err := replyContext(ctx, &c.commandBase, "Active prefixes: "+strings.Join(common.CommandPrefixes(c.engine), " ")); err != nil {
			return model.FAILED, err
		}
		return model.SUCCESSFUL, nil
	}
	controller, ok := c.engine.(common.PrefixController)
	if !ok {
		return model.FAILED, fmt.Errorf("prefix controller is unavailable")
	}
	prefixes, err := common.NormalizePrefixes(arguments)
	if err != nil {
		return model.FAILED, replyContext(ctx, &c.commandBase, err.Error())
	}
	previous, err := controller.UpdatePrefix(prefixes...)
	if err != nil {
		return model.FAILED, err
	}
	if err := replyContext(ctx, &c.commandBase, "prefixes changed from "+previous+" to "+strings.Join(prefixes, " ")); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
