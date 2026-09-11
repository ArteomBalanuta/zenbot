package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/model"
)

type lastonlineCommand struct{ commandBase }

func (c *lastonlineCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) == 0 {
		if err := replyContext(ctx, &c.commandBase, "\\n Example: "+c.engine.GetPrefix()+"lastseen merc"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	target := strings.TrimSpace(arguments[0])
	if strings.HasPrefix(target, "@") {
		target = strings.TrimSpace(strings.TrimPrefix(target, "@"))
	}
	if target == "" {
		if err := replyContext(ctx, &c.commandBase, "\\n Example: "+c.engine.GetPrefix()+"lastseen merc"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	services := bundle(c.engine)
	if services == nil || services.Users == nil || (services.Users.Queries == nil && services.Users.LastSeen == nil) {
		return model.FAILED, fmt.Errorf("last-online user queries unavailable")
	}
	text, err := services.Users.LastOnline(ctx, target)
	if err != nil {
		return model.FAILED, err
	}
	if err := observeAndReply(ctx, &c.commandBase, text, c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
