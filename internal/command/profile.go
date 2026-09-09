package command

import (
	"context"
	"encoding/json"
	"strings"

	"zenbot/internal/model"
)

type profileCommand struct {
	commandBase
	flair bool
}

func (c *profileCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	kind, example := "color", "color merc 00ff00"
	if c.flair {
		kind, example = "flair", "flair merc trusted"
	}
	if len(a) < 2 {
		reply(&c.commandBase, "\\n Example: "+c.engine.GetPrefix()+example)
		return model.FAILED, nil
	}
	target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(a[0]), "@"))
	if target == "" {
		reply(&c.commandBase, "\\n Example: "+c.engine.GetPrefix()+example)
		return model.FAILED, nil
	}
	active := false
	for u := range *c.engine.GetActiveUsers() {
		if u != nil && strings.EqualFold(u.Name, target) {
			active = true
			break
		}
	}
	if !active {
		reply(&c.commandBase, "User "+target+" is not in the room, "+kind+" was not applied.")
		return model.FAILED, nil
	}
	payload := map[string]string{"cmd": "force" + kind, "nick": target, kind: strings.TrimSpace(a[1])}
	body, _ := json.Marshal(payload)
	c.engine.SendRawMessage(string(body))
	if c.flair {
		reply(&c.commandBase, "\\n Flair set successfully!")
	}
	return model.SUCCESSFUL, nil
}
