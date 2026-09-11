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
		if _, err := c.engine.SendChatMessage(c.message.Name, "Example: "+c.engine.GetPrefix()+"active 8Wotmg", activityWhisper(c.message)); err != nil {
			return model.FAILED, err
		}
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
	text := "Stats: \\n" + result
	if err := observeAndReply(ctx, &c.commandBase, text, activityWhisper(c.message)); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func activityWhisper(message *model.ChatMessage) bool {
	return message.Whisper || message.IsWhisper || message.Type == "whisper"
}
