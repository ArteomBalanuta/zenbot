package command

import (
	"context"

	"zenbot/internal/model"
)

type captchaCommand struct{ commandBase }

func (c *captchaCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	argument, present := firstModerationArgument(c.message)
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	switch {
	case !present || argument == "on":
		err = operations.EnableCaptcha(ctx)
		if err == nil {
			reply(&c.commandBase, " Captcha enabled!")
		}
	case argument == "off":
		err = operations.DisableCaptcha(ctx)
		if err == nil {
			reply(&c.commandBase, " Captcha disabled!")
		}
	default:
		reply(&c.commandBase, c.engine.GetPrefix()+"captcha [on|off]")
		return model.FAILED, nil
	}
	if err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
