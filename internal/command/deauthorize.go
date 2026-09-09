package command

import (
	"context"
	"encoding/json"
	"strings"

	"zenbot/internal/model"
)

type deauthorizeCommand struct{ commandBase }

func (c *deauthorizeCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) == 0 || strings.TrimSpace(arguments[0]) == "" {
		reply(&c.commandBase, " example: "+c.engine.GetPrefix()+"deauth cmdTV+")
		return model.FAILED, nil
	}
	trip := strings.TrimSpace(arguments[0])
	payload, _ := json.Marshal(map[string]string{"cmd": "deauthtrip", "trip": trip})
	c.engine.SendRawMessage(string(payload))
	reply(&c.commandBase, " deauthorized trip: "+trip)
	return model.SUCCESSFUL, nil
}
