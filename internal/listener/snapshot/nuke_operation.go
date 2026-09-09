package snapshot

import (
	"encoding/json"
	"fmt"
	"time"

	"zenbot/internal/util"
)

// NukeRoomOperation applies the snapshot-derived moderation actions through
// the temporary room session owned by the coordinator.
type NukeRoomOperation struct {
	delay time.Duration
}

// NewNukeRoomOperation uses Saturn's 200 ms inter-ban delay unless a test or
// caller explicitly supplies a delay.
func NewNukeRoomOperation(delay ...time.Duration) NukeRoomOperation {
	operation := NukeRoomOperation{delay: 200 * time.Millisecond}
	if len(delay) > 0 {
		operation.delay = delay[0]
	}
	return operation
}

func (o NukeRoomOperation) Apply(ctx RoomSnapshotContext, snapshot Snapshot) (OperationResult, error) {
	for _, user := range snapshot.Users {
		if user == nil {
			return Failed("Failed to nuke " + ctx.TargetChannel), nil
		}
		nick, err := util.NormalizeNickTarget(&user.Name)
		if err != nil {
			return Failed("Failed to nuke " + ctx.TargetChannel), nil
		}
		if err := sendNukeRaw(ctx, map[string]string{"cmd": "ban", "nick": nick}); err != nil {
			return Failed("Failed to nuke " + ctx.TargetChannel), nil
		}
		if o.delay > 0 {
			time.Sleep(o.delay)
		}
	}
	if err := sendNukeRaw(ctx, map[string]string{"cmd": "lockroom"}); err != nil {
		return Failed("Failed to nuke " + ctx.TargetChannel), nil
	}
	return Success(), nil
}

func sendNukeRaw(ctx RoomSnapshotContext, payload map[string]string) error {
	if ctx.SendRaw == nil {
		return fmt.Errorf("snapshot raw sender is not configured")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return ctx.SendRaw(string(raw))
}
