package command

import (
	"context"
	"encoding/json"
	"strings"

	"zenbot/internal/model"
)

type unmuteCommand struct{ commandBase }

func (c *unmuteCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 || strings.TrimSpace(a[0]) == "" {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"unmute jJ4M4fsECSazzlj")
		return model.FAILED, nil
	}
	hash := strings.TrimSpace(a[0])
	payload, _ := json.Marshal(map[string]string{"cmd": "unmute", "hash": hash})
	c.engine.SendRawMessage(string(payload))
	reply(&c.commandBase, hash+" has been unmuted")
	return model.SUCCESSFUL, nil
}
