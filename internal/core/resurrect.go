package core

import (
	"context"

	"zenbot/internal/common"
)

// MoveFromServingRoom selects an exact managed replica before the host and
// performs the typed move on that source only.
func (e *EngineImpl) MoveFromServingRoom(ctx context.Context, from string, target common.NickTarget, to common.Channel) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if e.replicaController != nil && e.replicaController.manager != nil {
		if replica, ok := e.replicaController.manager.ManagedEngines()[from]; ok {
			if operations, ok := replica.(common.ModerationOperations); ok {
				return true, operations.KickNickTo(ctx, target, to)
			}
		}
	}
	if from == e.Channel {
		return true, e.KickNickTo(ctx, target, to)
	}
	return false, nil
}
