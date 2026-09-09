package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type automoveCommand struct{ commandBase }

func (c *automoveCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) > 0 && strings.EqualFold(a[0], "on") {
		controller, ok := c.engine.(common.AutoMoveController)
		if !ok {
			return model.FAILED, fmt.Errorf("automove controller is not configured")
		}
		snapshot, err := controller.EnableAutoMove(ctx)
		if err != nil {
			return model.FAILED, err
		}
		reply(&c.commandBase, " "+c.engine.GetPrefix()+"automove is enabled")
		_ = snapshot
		return model.SUCCESSFUL, nil
	}
	if len(a) > 0 && strings.EqualFold(a[0], "off") {
		controller, ok := c.engine.(common.AutoMoveController)
		if !ok {
			return model.FAILED, fmt.Errorf("automove controller is not configured")
		}
		if _, err := controller.DisableAutoMove(ctx); err != nil {
			return model.FAILED, err
		}
		reply(&c.commandBase, " "+c.engine.GetPrefix()+"automove is disabled")
		return model.SUCCESSFUL, nil
	}
	if len(a) == 2 {
		controller, ok := c.engine.(common.AutoMoveController)
		if !ok {
			return model.FAILED, fmt.Errorf("automove controller is not configured")
		}
		snapshot, err := controller.ConfigureAutoMove(a[0], a[1])
		if err != nil {
			return model.FAILED, err
		}
		reply(&c.commandBase, fmt.Sprintf("Set source channel: %v , destination channel: %s. Make sure bot's REPLICA is serving source channels.", snapshot.Sources, snapshot.Destination))
		return model.SUCCESSFUL, nil
	}
	snapshot := common.AutoMoveSnapshot{Sources: []string{"purgatory"}, Destination: "lounge"}
	if controller, ok := c.engine.(common.AutoMoveController); ok {
		snapshot = controller.AutoMoveSnapshot()
	}
	reply(&c.commandBase, c.engine.GetPrefix()+"automove [on|off]")
	reply(&c.commandBase, fmt.Sprintf("Current status: %t , Source rooms: %v , Destination room: %s", snapshot.Enabled, snapshot.Sources, snapshot.Destination))
	reply(&c.commandBase, "To set source: hell, destination: heaven - use: "+c.engine.GetPrefix()+"automove hell heaven")
	return model.FAILED, nil
}
