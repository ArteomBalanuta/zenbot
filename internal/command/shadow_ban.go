package command

import (
	"context"
	"fmt"
	"sort"
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

type shadowBanCommand struct{ commandBase }

func (c *shadowBanCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	mode, target, ok := parseShadowBanSelection(args(c.message))
	if !ok {
		if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"shadowban merc"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	if mode == "contains" {
		return c.shadowBanContaining(ctx, target)
	}
	normalized, err := util.NormalizeNickTarget(&target)
	if err != nil {
		return model.FAILED, err
	}
	return c.shadowBanSingle(ctx, normalized)
}

func (c *shadowBanCommand) shadowBanContaining(ctx context.Context, pattern string) (model.Status, error) {
	users := c.engine.GetActiveUsers()
	if users == nil {
		return model.FAILED, nil
	}
	candidateNames := make([]string, 0, len(*users))
	for user := range *users {
		if user == nil || !strings.Contains(user.Name, pattern) {
			continue
		}
		candidateNames = append(candidateNames, user.Name)
	}
	sort.SliceStable(candidateNames, func(i, j int) bool {
		left, right := strings.ToLower(candidateNames[i]), strings.ToLower(candidateNames[j])
		if left == right {
			return candidateNames[i] < candidateNames[j]
		}
		return left < right
	})
	selected := make([]*model.User, 0, len(candidateNames))
	seen := make(map[string]struct{}, len(candidateNames))
	for _, name := range candidateNames {
		current, err := activeModerationTarget(c.engine, name)
		if err != nil {
			return model.FAILED, err
		}
		if current == nil {
			continue
		}
		canonical := strings.ToLower(current.Name)
		if _, duplicate := seen[canonical]; duplicate {
			continue
		}
		seen[canonical] = struct{}{}
		selected = append(selected, current)
	}
	if len(selected) == 0 {
		return model.FAILED, nil
	}
	for _, current := range selected {
		if err := ctx.Err(); err != nil {
			return model.FAILED, err
		}
		if err := c.shadowBanPresent(ctx, current); err != nil {
			return model.FAILED, err
		}
		if err := ctx.Err(); err != nil {
			return model.FAILED, err
		}
	}
	return model.SUCCESSFUL, nil
}

func (c *shadowBanCommand) shadowBanSingle(ctx context.Context, target string) (model.Status, error) {
	user, err := activeModerationTarget(c.engine, target)
	if err != nil {
		return model.FAILED, err
	}
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if user != nil {
		if err := c.shadowBanPresent(ctx, user); err != nil {
			return model.FAILED, err
		}
		if err := ctx.Err(); err != nil {
			return model.FAILED, err
		}
		if err := replyContext(ctx, &c.commandBase, fmt.Sprintf("shadow_banned: %s trip: %s hash: %s", target, user.Trip, user.Hash)); err != nil {
			return model.FAILED, err
		}
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
	if err := replyContext(ctx, &c.commandBase, "banned: "+target); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func parseShadowBanSelection(arguments []string) (string, string, bool) {
	switch {
	case len(arguments) == 1 && arguments[0] != "-c":
		return "exact", arguments[0], true
	case len(arguments) == 2 && arguments[0] == "-c" && arguments[1] != "-c":
		return "contains", arguments[1], true
	default:
		return "", "", false
	}
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
