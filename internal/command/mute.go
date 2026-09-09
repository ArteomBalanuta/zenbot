package command

import (
	"context"
	"encoding/json"
	"strings"

	"zenbot/internal/model"
)

type muteCommand struct{ commandBase }

func (c *muteCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"mute merc")
		return model.FAILED, nil
	}
	target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(a[0]), "@"))
	if target == "" {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"mute merc")
		return model.FAILED, nil
	}
	var user *model.User
	for u := range *c.engine.GetActiveUsers() {
		if u != nil && strings.EqualFold(u.Name, target) {
			user = u
			break
		}
	}
	if user == nil {
		reply(&c.commandBase, target+" is not in the room")
		return model.FAILED, nil
	}
	payload, _ := json.Marshal(map[string]string{"cmd": "mute", "nick": target})
	c.engine.SendRawMessage(string(payload))
	reply(&c.commandBase, target+" "+user.Hash+" has been muted")
	return model.SUCCESSFUL, nil
}
