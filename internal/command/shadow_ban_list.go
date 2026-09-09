package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/model"
)

type shadowBanListCommand struct{ commandBase }

func (c *shadowBanListCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	b := bundle(c.engine)
	if b == nil || b.ShadowBans == nil {
		return model.FAILED, fmt.Errorf("shadow-ban persistence unavailable")
	}
	rows, err := b.ShadowBans.ListShadowBans(ctx)
	if err != nil {
		return model.FAILED, err
	}
	if len(rows) == 0 {
		reply(&c.commandBase, "No users has been banned.")
		return model.SUCCESSFUL, nil
	}
	var out strings.Builder
	for _, r := range rows {
		trip := r.Trip
		if trip == "" {
			trip = "------"
		}
		out.WriteString(r.Hash + " - " + trip + " - " + r.Name + "\\n")
	}
	reply(&c.commandBase, "Banned hashes, trips, names: \\n"+out.String())
	return model.SUCCESSFUL, nil
}
