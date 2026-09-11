package snapshot

import (
	"encoding/json"
	"errors"
	"testing"

	"zenbot/internal/model"
)

func TestRemoteMessageOperationAllIsMeIsEmpty(t *testing.T) {
	called := 0
	result, err := NewRemoteMessageOperation("formatted body").Apply(RoomSnapshotContext{
		TargetChannel: "target",
		SendRaw:       func(string) error { called++; return nil },
	}, Snapshot{Users: []*model.User{{Name: "temporary", Isme: true}}})
	if err != nil || result.Outcome != OutcomeEmpty || result.Reply != " target is empty" || len(result.Data) != 0 || called != 0 {
		t.Fatalf("result=%+v err=%v sends=%d", result, err, called)
	}
}

func TestRemoteMessageOperationSendsOneRawChat(t *testing.T) {
	var raw string
	result, err := NewRemoteMessageOperation("formatted \"body\"").Apply(RoomSnapshotContext{
		TargetChannel: "target",
		SendRaw:       func(value string) error { raw = value; return nil },
	}, Snapshot{Users: []*model.User{{Name: "temporary", Isme: true}, {Name: "member"}}})
	if err != nil || result.Outcome != OutcomeSuccess || result.Reply != "sent successfully." || len(result.Data) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(raw), &payload); err != nil || payload["cmd"] != "chat" || payload["nick"] != "*" || payload["text"] != "formatted \"body\"" {
		t.Fatalf("raw=%q payload=%v err=%v", raw, payload, err)
	}
}

func TestRemoteMessageOperationReturnsRawSendErrorWithoutRetry(t *testing.T) {
	sendErr := errors.New("raw failed")
	called := 0
	result, err := NewRemoteMessageOperation("body").Apply(RoomSnapshotContext{SendRaw: func(string) error { called++; return sendErr }}, Snapshot{Users: []*model.User{{Name: "member"}}})
	if !errors.Is(err, sendErr) || result.Outcome != OutcomeFailed || called != 1 {
		t.Fatalf("result=%+v err=%v sends=%d", result, err, called)
	}
}
