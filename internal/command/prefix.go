package command

import (
	"context"
	"strings"

	"zenbot/internal/model"
)

// prefixSetter is intentionally optional so existing Engine implementations
// remain source-compatible while engines that own replica topology can update
// their whole serving group atomically.
type prefixSetter interface {
	SetPrefix(string)
}

type prefixCommand struct{ commandBase }

func (c *prefixCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	args := args(c.message)
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		reply(&c.commandBase, "prefix $")
		return model.FAILED, nil
	}
	setter, ok := c.engine.(prefixSetter)
	if !ok {
		return model.FAILED, nil
	}
	previous := c.engine.GetPrefix()
	next := strings.TrimSpace(args[0])
	setter.SetPrefix(next)
	reply(&c.commandBase, "prefix changed from "+previous+" to "+next)
	return model.SUCCESSFUL, nil
}
