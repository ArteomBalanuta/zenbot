package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/model"
)

// sqlCommand retains the source parser's case-sensitive literal "sql " split.
// Rendering and replies remain deferred to later tracers.
type sqlCommand struct{ commandBase }

func (c *sqlCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}

	parts := strings.Split(c.message.Text, "sql ")
	if len(parts) < 2 || parts[1] == "" {
		// Java String.split discards a trailing empty payload. Return an internal
		// failure instead of letting the equivalent index access panic.
		return model.FAILED, fmt.Errorf("sql source payload is malformed")
	}
	query := strings.ReplaceAll(parts[1], `\n`, "\n")

	b := bundle(c.engine)
	if b == nil || b.SQLCommand == nil {
		return model.FAILED, fmt.Errorf("raw SQL query capability is not configured")
	}
	table, err := b.SQLCommand.Query(ctx, query)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return model.FAILED, ctxErr
	}
	if err != nil {
		reply(&c.commandBase, "Result: \\n"+err.Error())
		return model.SUCCESSFUL, nil
	}
	reply(&c.commandBase, "Result: \\n"+renderSaturnSQLTable(table))
	return model.SUCCESSFUL, nil
}
