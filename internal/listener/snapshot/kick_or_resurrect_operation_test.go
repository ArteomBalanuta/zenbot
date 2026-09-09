package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"zenbot/internal/model"
)

func TestKickOrResurrectOperationMovesNormalizedPresentTarget(t *testing.T) {
	var raws []string
	operation := NewKickOrResurrectOperation("@Alice")
	result, err := operation.Apply(RoomSnapshotContext{DestinationChannel: "destination", SendRaw: func(raw string) error {
		raws = append(raws, raw)
		return nil
	}}, Snapshot{Users: []*model.User{{Name: "aLiCe"}}})
	if err != nil || result.Outcome != OutcomeSuccess || len(raws) != 1 {
		t.Fatalf("result=%+v err=%v raws=%v", result, err, raws)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(raws[0]), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["cmd"] != "kick" || payload["nick"] != "Alice" || payload["to"] != "destination" || len(payload) != 3 {
		t.Fatalf("payload=%v", payload)
	}
}

func TestKickOrResurrectOperationReportsAbsentTargetWithoutRaw(t *testing.T) {
	called := false
	result, err := NewKickOrResurrectOperation("Alice").Apply(RoomSnapshotContext{SendRaw: func(string) error {
		called = true
		return nil
	}}, Snapshot{Users: []*model.User{nil, {Name: "Bob"}}})
	if err != nil || result.Outcome != OutcomeAbsentTarget || result.Reply != " Alice isn't in the room" || called {
		t.Fatalf("result=%+v err=%v called=%t", result, err, called)
	}
}

func TestKickOrResurrectOperationReturnsOperationErrorForUnavailableTemporarySender(t *testing.T) {
	result, err := NewKickOrResurrectOperation("Alice").Apply(RoomSnapshotContext{}, Snapshot{Users: []*model.User{{Name: "Alice"}}})
	if result.Outcome != OutcomeFailed || err == nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if !errors.Is(err, context.Canceled) && err.Error() != "snapshot raw sender is not configured" {
		t.Fatalf("error=%v", err)
	}
}
