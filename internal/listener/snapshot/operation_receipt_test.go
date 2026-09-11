package snapshot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"zenbot/internal/model"
)

func TestNukeRetainsPartialSendCountAndWriteFailure(t *testing.T) {
	calls := 0
	writeErr := errors.New("ambiguous write")
	r, err := NewNukeRoomOperation(0).Apply(RoomSnapshotContext{SendRaw: func(string) error {
		calls++
		if calls == 2 {
			return writeErr
		}
		return nil
	}}, Snapshot{Users: []*model.User{{Name: "alice"}, {Name: "bob"}}})
	if !errors.Is(err, writeErr) || r.ActionCount != 1 || !r.OutcomeUnknown || r.Outcome != OutcomeFailed {
		t.Fatalf("result=%+v err=%v", r, err)
	}
}

func TestNukeCanceledWriteIsUnknownButPreCanceledExecutionIsNot(t *testing.T) {
	for _, preCanceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if preCanceled {
			cancel()
		}
		calls := 0
		r, err := NewNukeRoomOperation(0).Apply(RoomSnapshotContext{Context: ctx, SendRaw: func(string) error { calls++; cancel(); return context.Canceled }}, Snapshot{})
		cancel()
		if !errors.Is(err, context.Canceled) || r.OutcomeUnknown == preCanceled || calls != map[bool]int{true: 0, false: 1}[preCanceled] {
			t.Fatalf("preCanceled=%t result=%+v err=%v calls=%d", preCanceled, r, err, calls)
		}
	}
}
func TestNukeCancellationStopsInterActionWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	began := time.Now()
	r, err := NewNukeRoomOperation(time.Second).Apply(RoomSnapshotContext{Context: ctx, SendRaw: func(string) error { calls++; cancel(); return nil }}, Snapshot{Users: []*model.User{{Name: "alice"}, {Name: "bob"}}})
	if !errors.Is(err, context.Canceled) || calls != 1 || r.ActionCount != 1 || time.Since(began) > 500*time.Millisecond {
		t.Fatalf("result=%+v err=%v calls=%d elapsed=%s", r, err, calls, time.Since(began))
	}
}
func TestResurrectRequestUsesActualSnapshotNickAndCountsSend(t *testing.T) {
	raw := ""
	r, err := NewKickOrResurrectOperation("alice").Apply(RoomSnapshotContext{SendRaw: func(s string) error { raw = s; return nil }}, Snapshot{Users: []*model.User{{Name: "ALIce"}}})
	if err != nil || r.ActionCount != 1 || !strings.Contains(raw, `"nick":"ALIce"`) {
		t.Fatalf("result=%+v err=%v raw=%s", r, err, raw)
	}
}
func TestRemoteMessageRejectsMissingSenderAndNilUser(t *testing.T) {
	for _, users := range [][]*model.User{{nil}, {{Name: "alice"}}} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("operation panicked: %v", r)
				}
			}()
			r, err := NewRemoteMessageOperation("hello").Apply(RoomSnapshotContext{}, Snapshot{Users: users})
			if err == nil || r.Outcome != OutcomeFailed {
				t.Fatalf("result=%+v err=%v", r, err)
			}
		}()
	}
}

func TestRemoteMessageEmptyBodyDoesNotWriteOrClaimDelivery(t *testing.T) {
	called := false
	r, err := NewRemoteMessageOperation(" \n ").Apply(RoomSnapshotContext{SendRaw: func(string) error { called = true; return nil }}, Snapshot{Users: []*model.User{{Name: "alice"}}})
	if err != nil || called || r.Outcome != OutcomeEmpty || r.ActionCount != 0 || r.DeliveryCount != 0 {
		t.Fatalf("result=%+v err=%v wrote=%t", r, err, called)
	}
}
