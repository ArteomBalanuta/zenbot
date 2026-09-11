package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/model"
)

// sqlCommand consumes the query body independently of the resolved command alias.
type sqlCommand struct{ commandBase }

func (c *sqlCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}

	_, query := splitCommandToken(c.message.Text)
	if strings.TrimSpace(query) == "" {
		return model.FAILED, fmt.Errorf("sql requires a query")
	}

	b := bundle(c.engine)
	if b == nil || b.SQLCommand == nil {
		return model.FAILED, fmt.Errorf("raw SQL query capability is not configured")
	}
	table, err := b.SQLCommand.Query(ctx, query)
	var text string
	if err == nil {
		text = "Result: \\n" + renderSaturnSQLTable(table)
		observeCommandData(&c.commandBase, commandTextObservation{Text: text}, c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper")
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return model.FAILED, ctxErr
	}
	if err != nil {
		return model.FAILED, err
	}
	if _, err := c.engine.SendChatMessage(c.message.Name, text, c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
