package command

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"zenbot/internal/model"
	"zenbot/internal/repository"
)

type lastonlineCommand struct{ commandBase }

func (c *lastonlineCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) == 0 {
		if err := replyContext(ctx, &c.commandBase, "\n Example: "+c.engine.GetPrefix()+"lastseen merc"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	target := strings.TrimSpace(arguments[0])
	// This is a nickname OR trip selector. The repository normalizes only
	// the nickname predicate while preserving the exact opaque trip operand.
	if target == "" || target == "@" {
		if err := replyContext(ctx, &c.commandBase, "\n Example: "+c.engine.GetPrefix()+"lastseen merc"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	services := bundle(c.engine)
	if services == nil || services.Users == nil || (services.Users.Queries == nil && services.Users.LastSeen == nil) {
		return model.FAILED, fmt.Errorf("last-online user queries unavailable")
	}
	text, err := services.Users.LastOnline(ctx, target)
	if errors.Is(err, repository.ErrNotFound) {
		text = "No public history found for nickname or trip: " + target + "."
	} else if err != nil {
		return model.FAILED, err
	}
	if err := observeAndReply(ctx, &c.commandBase, text, c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper"); err != nil {
		return model.FAILED, err
	}
	// Deliver the explanation without turning a missing record into a found
	// result for callers such as the agent gateway.
	if err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
