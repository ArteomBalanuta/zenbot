package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/model"
	"zenbot/internal/repository"
)

type shadowBanListCommand struct{ commandBase }

func (c *shadowBanListCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	service, err := shadowBanService(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	records, err := service.List(ctx)
	if err != nil {
		return model.FAILED, err
	}
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if len(records) == 0 {
		reply(&c.commandBase, "No users has been banned.")
		return model.SUCCESSFUL, nil
	}
	reply(&c.commandBase, "Banned hashes, trips, names: \\n"+formatShadowBanRecords(records))
	return model.SUCCESSFUL, nil
}

func formatShadowBanRecords(records []repository.ShadowBanRecord) string {
	var out strings.Builder
	for _, record := range records {
		trip := record.Trip
		if trip == "" {
			trip = "------"
		}
		fmt.Fprintf(&out, "%s - %s - %s\\n", record.Hash, trip, record.Name)
	}
	return out.String()
}
