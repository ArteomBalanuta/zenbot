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

func (c *dbzCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
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
		reply(&c.commandBase, "Successfully registered character: "+author)
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
		n, err := strconv.Atoi(strings.TrimSpace(a[0]))
		if err != nil || n <= 0 {
			reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"daddstr amount")
			return model.FAILED, nil
		}
		free, err := b.DBZ.FreeStats(ctx, author)
		if err != nil {
			return model.FAILED, err
		}
		if free <= 0 {
			reply(&c.commandBase, "You don't have free stats. Level up!")
			return model.SUCCESSFUL, nil
		}
		_ = b.DBZ.AddStrength(ctx, author, n)
		reply(&c.commandBase, strconv.Itoa(n))
	case "dfight":
		if len(a) == 0 {
			reply(&c.commandBase, "dfight enemy")
			return model.FAILED, nil
		}
		b.DBZ.Fight(strings.TrimSpace(a[0]))
		_ = b.DBZ.LevelUp(ctx, author)
		reply(&c.commandBase, "Gz. Enemy has been slain. Your leveled up! Granted 5 free stats!")
	case "dbzhelp":
		reply(&c.commandBase, "This is a DBZ universe text based game.\\nMain mechanics:\\n/train, - training your char in order to level up and gain point (stats)\\n/fight <nick>, - fight against a player\\n/claim - claim an item that just spawned\\n \\n/stats - displays character stats\\n/strength <int> - add a point into str\\n/agility <int> - add a point into agility\\n/vitality <int> - add a point into vitality\\n/energy <int> - add a point into energy\\n")
	case "dspawn":
		if len(a) == 0 {
			reply(&c.commandBase, "dspawn enemy")
			return model.FAILED, nil
		}
		enemy := strings.TrimSpace(a[0])
		b.DBZ.SpawnEnemy(enemy)
		reply(&c.commandBase, "spawned enemy: "+enemy)
	}
	return model.SUCCESSFUL, nil
}
func (c *dbzCommand) NewInstance(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	return &dbzCommand{commandBase{engine: e, message: m, role: model.REGULAR, aliases: c.aliases}}
}
