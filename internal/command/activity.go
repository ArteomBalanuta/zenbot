package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/model"
)

type activityCommand struct{ commandBase }

func (c *activityCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) == 0 || strings.TrimSpace(arguments[0]) == "" {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"active 8Wotmg")
		return model.FAILED, nil
	}
	services := bundle(c.engine)
	if services == nil || services.Activity == nil || services.Activity.Repo == nil {
		return model.FAILED, fmt.Errorf("activity service unavailable")
	}
	result, err := services.Activity.Stats(ctx, strings.TrimSpace(arguments[0]))
	if err != nil {
		return model.FAILED, err
	}
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, "Stats: \\n"+result)
	return model.SUCCESSFUL, nil
}
