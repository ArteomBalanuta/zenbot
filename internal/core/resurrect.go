package core

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/common"
	"zenbot/internal/repository"
)

// MoveFromServingRoom selects an exact managed replica before the host and
// performs the typed move on that source only.
func (e *EngineImpl) MoveFromServingRoom(ctx context.Context, from, selector string, to common.Channel) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if e.replicaController != nil && e.replicaController.manager != nil {
		if replica, ok := e.replicaController.manager.ManagedEngines()[from]; ok {
			return moveActiveUserFromSource(ctx, replica, selector, to)
		}
	}
	if from == e.Channel {
		return moveActiveUserFromSource(ctx, e, selector, to)
	}
	return false, nil
}

func moveActiveUserFromSource(ctx context.Context, source common.Engine, selector string, to common.Channel) (bool, error) {
	user := source.GetActiveUserByName(selector)
	if err := ctx.Err(); err != nil {
		return true, err
	}
	if user == nil || strings.TrimSpace(user.Name) == "" {
		return true, fmt.Errorf("move target %q is not active on source: %w", selector, repository.ErrNotFound)
	}
	operations, ok := source.(common.ModerationOperations)
	if !ok {
		return true, fmt.Errorf("source moderation operations are unavailable")
	}
	return true, operations.KickNickTo(ctx, common.NickTarget(user.Name), to)
}
