package command

import (
	"context"
	"encoding/json"
	"strings"

	"zenbot/internal/model"
)

type captchaCommand struct{ commandBase }

func (c *captchaCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	mode := "on"
	if a := args(c.message); len(a) > 0 {
		mode = strings.TrimSpace(a[0])
	}
	cmd, replyText := "enablecaptcha", " Captcha enabled!"
	if mode == "off" {
		cmd, replyText = "disablecaptcha", " Captcha disabled!"
	}
	if mode != "on" && mode != "off" {
		reply(&c.commandBase, c.engine.GetPrefix()+"captcha [on|off]")
		return model.FAILED, nil
	}
	payload, _ := json.Marshal(map[string]string{"cmd": cmd})
	c.engine.SendRawMessage(string(payload))
	reply(&c.commandBase, replyText)
	return model.SUCCESSFUL, nil
}
