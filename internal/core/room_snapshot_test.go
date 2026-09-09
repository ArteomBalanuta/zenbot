package core

import (
	"testing"
	"time"

	"zenbot/internal/listener/snapshot"
)

type roomSnapshotSessionStub struct{ id string }

func (s *roomSnapshotSessionStub) ID() string           { return s.id }
func (s *roomSnapshotSessionStub) Start() error         { return nil }
func (s *roomSnapshotSessionStub) Close() error         { return nil }
func (s *roomSnapshotSessionStub) Flush() error         { return nil }
func (s *roomSnapshotSessionStub) SendRaw(string) error { return nil }

func validRoomSnapshotRequest() snapshot.RoomSnapshotRequest {
	return snapshot.RoomSnapshotRequest{
		WorkflowID:    "wf-1",
		Author:        "admin",
		SourceChannel: "source",
		TargetChannel: "target",
		Operation: snapshot.OperationFunc(func(snapshot.RoomSnapshotContext, snapshot.Snapshot) (snapshot.OperationResult, error) {
			return snapshot.Success(), nil
		}),
	}
}

func TestEngineSubmitRoomSnapshotRequiresInstalledCoordinator(t *testing.T) {
	e := &EngineImpl{}

	if err := e.SubmitRoomSnapshot(validRoomSnapshotRequest()); err == nil || err.Error() != "room snapshot coordinator is not configured" {
		t.Fatalf("SubmitRoomSnapshot() error = %v, want missing coordinator configuration error", err)
	}
}

func TestEngineSubmitRoomSnapshotForwardsOneValidRequestToInstalledCoordinator(t *testing.T) {
	created := 0
	coordinator := snapshot.NewRoomSnapshotCoordinator(snapshot.SessionFactoryFunc(func(snapshot.RoomSnapshotRequest, snapshot.SnapshotSink) (snapshot.Session, error) {
		created++
		return &roomSnapshotSessionStub{id: "temporary-1"}, nil
	}), nil, func(payload string) (snapshot.Snapshot, error) { return snapshot.Parse(payload, false) }, time.Second)
	e := &EngineImpl{}
	e.InstallRoomSnapshotCoordinator(coordinator)

	if err := e.SubmitRoomSnapshot(validRoomSnapshotRequest()); err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatalf("temporary sessions created = %d, want 1", created)
	}
}
