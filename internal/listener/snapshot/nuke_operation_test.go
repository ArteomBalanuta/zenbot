package snapshot

import (
	"encoding/json"
	"testing"
	"time"

	"zenbot/internal/model"
)

func TestNukeRoomOperationDoesNotSendLockRequestWhenABanRequestFails(t *testing.T) {
	var payloads []map[string]string
	operation := NewNukeRoomOperation(0)
	result, err := operation.Apply(RoomSnapshotContext{
		TargetChannel: "hotlinks",
		SendRaw: func(raw string) error {
			payload := map[string]string{}
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				return err
			}
			payloads = append(payloads, payload)
			return assertNukeBanFailure(payload)
		},
	}, Snapshot{Users: []*model.User{{Name: "alice"}, {Name: "bob"}}})
	if err == nil || result.Outcome != OutcomeFailed || result.Reply != "Failed to nuke hotlinks" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(payloads) != 1 || !mapsEqual(payloads[0], map[string]string{"cmd": "ban", "nick": "alice"}) {
		t.Fatalf("payloads=%v", payloads)
	}
}

func assertNukeBanFailure(payload map[string]string) error {
	if payload["cmd"] == "ban" {
		return errNukeRawFailure{}
	}
	return nil
}

type errNukeRawFailure struct{}

func (errNukeRawFailure) Error() string { return "raw ban failed" }

func TestNukeRoomOperationSendsBanRequestsForLiteralSnapshotUsersBeforeLockRequest(t *testing.T) {
	var rawPayloads []map[string]string
	operation := NewNukeRoomOperation(0)
	result, err := operation.Apply(RoomSnapshotContext{
		TargetChannel: "hotlinks",
		SendRaw: func(raw string) error {
			payload := map[string]string{}
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				return err
			}
			rawPayloads = append(rawPayloads, payload)
			return nil
		},
	}, Snapshot{Users: []*model.User{{Name: "@alice"}, {Name: " bob "}}})
	if err != nil || result.Outcome != OutcomeSuccess {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	want := []map[string]string{{"cmd": "ban", "nick": "@alice"}, {"cmd": "ban", "nick": " bob "}, {"cmd": "lockroom"}}
	if len(rawPayloads) != len(want) {
		t.Fatalf("payload count=%d, want %d: %#v", len(rawPayloads), len(want), rawPayloads)
	}
	for i := range want {
		if !mapsEqual(rawPayloads[i], want[i]) {
			t.Fatalf("payload[%d]=%v, want %v", i, rawPayloads[i], want[i])
		}
	}
}

func TestNukeRoomOperationUses200MillisecondDelayBetweenEveryRequest(t *testing.T) {
	var sentAt []time.Time
	operation := NewNukeRoomOperation()
	result, err := operation.Apply(RoomSnapshotContext{
		TargetChannel: "hotlinks",
		SendRaw: func(string) error {
			sentAt = append(sentAt, time.Now())
			return nil
		},
	}, Snapshot{Users: []*model.User{{Name: "alice"}, {Name: "bob"}}})
	if err != nil || result.Outcome != OutcomeSuccess {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(sentAt) != 3 {
		t.Fatalf("actions=%d, want 3", len(sentAt))
	}
	for i := 1; i < len(sentAt); i++ {
		if gap := sentAt[i].Sub(sentAt[i-1]); gap < 175*time.Millisecond {
			t.Fatalf("gap[%d]=%s, want approximately 200ms", i, gap)
		}
	}
}

func mapsEqual(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			return false
		}
	}
	return true
}
