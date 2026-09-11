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
			if err := replyContext(ctx, &c.commandBase, " Captcha enabled!"); err != nil {
				return model.FAILED, err
			}
		}
	case argument == "off":
		err = operations.DisableCaptcha(ctx)
		if err == nil {
			if err := replyContext(ctx, &c.commandBase, " Captcha disabled!"); err != nil {
				return model.FAILED, err
			}
		}
	default:
		if err := replyContext(ctx, &c.commandBase, c.engine.GetPrefix()+"captcha [on|off]"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	if err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
