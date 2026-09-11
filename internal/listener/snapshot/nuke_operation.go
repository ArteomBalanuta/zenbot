package snapshot

import (
	"encoding/json"
	"fmt"
	"time"

	"zenbot/internal/util"
)

// NukeRoomOperation submits snapshot-derived moderation requests through the
// temporary room session owned by the coordinator.
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
	result := Success()
	fail := func(err error, unknown bool) (OperationResult, error) {
		result.Outcome = OutcomeFailed
		result.Reply = "Failed to nuke " + ctx.TargetChannel
		result.OutcomeUnknown = unknown
		return result, err
	}
	for _, user := range snapshot.Users {
		if err := executionContext(ctx).Err(); err != nil {
			return fail(err, false)
		}
		if user == nil {
			return fail(fmt.Errorf("snapshot contains nil user"), false)
		}
		nick, err := util.NormalizeNickTarget(&user.Name)
		if err != nil {
			return fail(err, false)
		}
		if err := sendNukeRaw(ctx, map[string]string{"cmd": "ban", "nick": nick}); err != nil {
			return fail(err, ctx.SendRaw != nil)
		}
		result.ActionCount++
		if o.delay > 0 {
			timer := time.NewTimer(o.delay)
			select {
			case <-executionContext(ctx).Done():
				timer.Stop()
				return fail(executionContext(ctx).Err(), false)
			case <-timer.C:
			}
		}
	}
	if err := executionContext(ctx).Err(); err != nil {
		return fail(err, false)
	}
	if err := sendNukeRaw(ctx, map[string]string{"cmd": "lockroom"}); err != nil {
		return fail(err, ctx.SendRaw != nil)
	}
	result.ActionCount++
	return result, nil
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
