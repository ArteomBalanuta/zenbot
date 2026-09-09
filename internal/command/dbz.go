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

const dbzHelpPayload = `This is a DBZ universe text based game.
Main mechanics:
/train, - training your char in order to level up and gain point (stats)
/fight <nick>, - fight against a player
/claim - claim an item that just spawned
\u2009
/stats - displays character stats
/strength <int> - add a point into str
/agility <int> - add a point into agility
/vitality <int> - add a point into vitality
/energy <int> - add a point into energy
`

func (c *dbzCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if c.canonical == "dbzhelp" {
		if _, err := c.engine.SendChatMessage("", dbzHelpPayload, false); err != nil {
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
		_ = b.DBZ.Register(ctx, author)
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
		free, err := b.DBZ.FreeStats(ctx, author)
		if err != nil {
			return model.FAILED, err
		}
		if free <= 0 {
			if _, err := c.engine.SendChatMessage("", "You don't have free stats. Level up!", false); err != nil {
				return model.FAILED, err
			}
			return model.SUCCESSFUL, nil
		}
		_ = b.DBZ.AddStrength(ctx, author, n)
		if _, err := c.engine.SendChatMessage("", strconv.Itoa(n), false); err != nil {
			return model.FAILED, err
		}
	case "dfight":
		if len(a) == 0 {
			reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"dfight enemy")
			return model.FAILED, nil
		}
		b.DBZ.Fight(strings.TrimSpace(a[0]))
		_ = b.DBZ.LevelUp(ctx, author)
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
	}
	return model.SUCCESSFUL, nil
}
func (c *dbzCommand) NewInstance(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	return &dbzCommand{commandBase{engine: e, message: m, role: model.REGULAR, aliases: c.aliases}}
}
