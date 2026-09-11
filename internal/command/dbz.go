package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type dbzCommand struct{ commandBase }

func dbzHelpPayload(prefix string) string {
	return `DBZ game commands:
` + prefix + `dbzregister - register your character
` + prefix + `dbzstats - show your character stats
` + prefix + `dspawn <enemy> - spawn an enemy
` + prefix + `dfight <enemy> - defeat a spawned enemy and gain one level plus 5 free stats
` + prefix + `dbzstr <amount> - spend free stats on strength
`
}

func (c *dbzCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if c.canonical == "dbzhelp" {
		if _, err := c.engine.SendChatMessage("", dbzHelpPayload(c.engine.GetPrefix()), false); err != nil {
			return model.FAILED, err
		}
		return model.SUCCESSFUL, nil
	}
	b := bundle(c.engine)
	if b == nil || b.DBZ == nil {
		return model.FAILED, fmt.Errorf("DBZ state unavailable")
	}
	a := args(c.message)
	author := c.message.Name
	switch c.canonical {
	case "dbzregister":
		if err := b.DBZ.Register(ctx, author); err != nil {
			return model.FAILED, err
		}
		if _, err := c.engine.SendChatMessage("", "Successfully registered character: "+author, false); err != nil {
			return model.FAILED, err
		}
	case "dbzstats":
		v, err := b.DBZ.StatsText(ctx, author)
		if err != nil {
			return model.FAILED, err
		}
		reply(&c.commandBase, v)
	case "dbzstr":
		if len(a) == 0 {
			reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"daddstr amount")
			return model.FAILED, nil
		}
		parsed, err := strconv.ParseInt(strings.TrimSpace(a[0]), 10, 32)
		if err != nil || parsed <= 0 {
			reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"daddstr amount")
			return model.FAILED, nil
		}
		n := int(parsed)
		if err := b.DBZ.AddStrength(ctx, author, n); err != nil {
			return model.FAILED, err
		}
		if _, err := c.engine.SendChatMessage("", strconv.Itoa(n), false); err != nil {
			return model.FAILED, err
		}
	case "dfight":
		if len(a) == 0 {
			reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"dfight enemy")
			return model.FAILED, nil
		}
		enemy := strings.TrimSpace(a[0])
		defeated, err := b.DBZ.Fight(ctx, author, enemy)
		if err != nil {
			return model.FAILED, err
		}
		if !defeated {
			return model.FAILED, fmt.Errorf("DBZ enemy %q not found", enemy)
		}
		if _, err := c.engine.SendChatMessage("", "Gz. Enemy has been slain. Your leveled up! Granted 5 free stats!", false); err != nil {
			return model.FAILED, err
		}
	case "dspawn":
		if len(a) == 0 {
			reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"dspawn enemy")
			return model.FAILED, nil
		}
		enemy := strings.TrimSpace(a[0])
		b.DBZ.SpawnEnemy(enemy)
		if _, err := c.engine.SendChatMessage("", "spawned enemy: "+enemy, false); err != nil {
			return model.FAILED, err
		}
	default:
		return model.FAILED, fmt.Errorf("no DBZ implementation for %q", c.canonical)
	}
	return model.SUCCESSFUL, nil
}
func (c *dbzCommand) NewInstance(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	return &dbzCommand{commandBase{canonical: c.canonical, engine: e, message: m, role: c.role, aliases: c.aliases}}
}
