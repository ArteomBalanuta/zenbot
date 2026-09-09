package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
	"zenbot/internal/util"
)

type shadowBanKicker interface {
	KickNick(context.Context, common.NickTarget) error
}

func shadowBanService(e common.Engine) (*service.ShadowBanService, error) {
	b := bundle(e)
	if b == nil || b.ShadowBans == nil {
		return nil, fmt.Errorf("shadow-ban service unavailable")
	}
	return b.ShadowBans, nil
}

type shadowBanListCommand struct{ commandBase }

func (c *shadowBanListCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	s, err := shadowBanService(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	records, err := s.List(ctx)
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

type shadowBanCommand struct{ commandBase }

func (c *shadowBanCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) == 0 {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"shadowban merc")
		return model.FAILED, nil
	}
	if len(arguments) > 1 && hasArgument(arguments, "-c") {
		return c.shadowBanContaining(ctx, arguments[1])
	}
	target, err := util.NormalizeNickTarget(&arguments[0])
	if err != nil {
		return model.FAILED, err
	}
	return c.shadowBanSingle(ctx, target)
}

func (c *shadowBanCommand) shadowBanContaining(ctx context.Context, pattern string) (model.Status, error) {
	for user := range *c.engine.GetActiveUsers() {
		if user == nil || !strings.Contains(user.Name, pattern) {
			continue
		}
		current := c.engine.GetActiveUserByName(user.Name)
		if current == nil {
			continue
		}
		if err := c.shadowBanPresent(ctx, current); err != nil {
			return model.FAILED, err
		}
	}
	return model.SUCCESSFUL, nil
}

func (c *shadowBanCommand) shadowBanSingle(ctx context.Context, target string) (model.Status, error) {
	if user := c.engine.GetActiveUserByName(target); user != nil {
		if err := c.shadowBanPresent(ctx, user); err != nil {
			return model.FAILED, err
		}
		if err := ctx.Err(); err != nil {
			return model.FAILED, err
		}
		reply(&c.commandBase, fmt.Sprintf("shadow_banned: %s trip: %s hash: %s", target, user.Trip, user.Hash))
		return model.SUCCESSFUL, nil
	}
	s, err := shadowBanService(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if err := s.Persist(ctx, repository.ShadowBanRecord{Name: target}); err != nil {
		return model.FAILED, err
	}
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, "banned: "+target)
	return model.SUCCESSFUL, nil
}

func (c *shadowBanCommand) shadowBanPresent(ctx context.Context, user *model.User) error {
	s, err := shadowBanService(c.engine)
	if err != nil {
		return err
	}
	if err := s.Persist(ctx, repository.ShadowBanRecord{Trip: user.Trip, Name: user.Name, Hash: user.Hash}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	kicker, ok := c.engine.(shadowBanKicker)
	if !ok {
		return fmt.Errorf("typed shadow-ban kick operation unavailable")
	}
	return kicker.KickNick(ctx, common.NickTarget(user.Name))
}

type unshadowBanCommand struct{ commandBase }

func (c *unshadowBanCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) == 0 {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"unban merc")
		return model.FAILED, nil
	}
	s, err := shadowBanService(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	if hasArgument(arguments, "-all") {
		records, err := s.List(ctx)
		if err != nil {
			return model.FAILED, err
		}
		if len(records) == 0 {
			reply(&c.commandBase, "No users has been banned.")
			return model.FAILED, nil
		}
		if err := s.RemoveAll(ctx); err != nil {
			return model.FAILED, err
		}
		if err := ctx.Err(); err != nil {
			return model.FAILED, err
		}
		reply(&c.commandBase, "Unbanned hashes, trips, nicks: \\n"+formatShadowBanRecords(records))
		return model.FAILED, nil
	}
	target := arguments[0]
	if err := s.Remove(ctx, target); err != nil {
		return model.FAILED, err
	}
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, " unbanned "+target)
	return model.SUCCESSFUL, nil
}

func hasArgument(arguments []string, value string) bool {
	for _, argument := range arguments {
		if argument == value {
			return true
		}
	}
	return false
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
