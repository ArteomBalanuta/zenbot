package command

import (
	"context"
	"encoding/json"
	"strings"

	"zenbot/internal/model"
)

type overflowCommand struct{ commandBase }

func (c *overflowCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 || strings.TrimSpace(a[0]) == "" {
		reply(&c.commandBase, "target nick isn't set! Example: "+c.engine.GetPrefix()+"shoot @merc")
		return model.FAILED, nil
	}
	target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(a[0]), "@"))
	if target == "" {
		reply(&c.commandBase, "target nick isn't set! Example: "+c.engine.GetPrefix()+"shoot @merc")
		return model.FAILED, nil
	}
	body, _ := json.Marshal(map[string]string{"cmd": "overflow", "nick": target})
	c.engine.SendRawMessage(string(body))
	return model.SUCCESSFUL, nil
}
