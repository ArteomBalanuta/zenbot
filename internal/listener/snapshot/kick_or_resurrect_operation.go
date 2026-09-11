package snapshot

import (
	"encoding/json"
	"fmt"

	"zenbot/internal/util"
)

// KickOrResurrectOperation submits the kick-to-room move request only after its
// source-room snapshot establishes that the normalized target is present.
type KickOrResurrectOperation struct{ target string }

func NewKickOrResurrectOperation(target string) KickOrResurrectOperation {
	normalized, _ := util.NormalizeNickTarget(&target)
	return KickOrResurrectOperation{target: normalized}
}

func (o KickOrResurrectOperation) Apply(ctx RoomSnapshotContext, source Snapshot) (OperationResult, error) {
	if err := executionContext(ctx).Err(); err != nil {
		return Failed(), err
	}
	for _, user := range source.Users {
		if user == nil || !util.SameNick(&user.Name, &o.target) {
			continue
		}
		if ctx.SendRaw == nil {
			return Failed(), fmt.Errorf("snapshot raw sender is not configured")
		}
		raw, err := json.Marshal(map[string]string{"cmd": "kick", "nick": user.Name, "to": ctx.DestinationChannel})
		if err != nil {
			return Failed(), err
		}
		if err := ctx.SendRaw(string(raw)); err != nil {
			result := Failed()
			result.OutcomeUnknown = true
			return result, err
		}
		result := Success()
		result.ActionCount = 1
		return result, nil
	}
	return Absent(" " + o.target + " isn't in the room"), nil
}
