package factory

import (
	"testing"
	"time"

	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
)

type snapshotSessionStub struct{ id string }

func (s *snapshotSessionStub) ID() string           { return s.id }
func (s *snapshotSessionStub) Start() error         { return nil }
func (s *snapshotSessionStub) Close() error         { return nil }
func (s *snapshotSessionStub) Flush() error         { return nil }
func (s *snapshotSessionStub) SendRaw(string) error { return nil }

func TestNewEngineWithOptionsInstallsRoomSnapshotCoordinator(t *testing.T) {
	created := 0
	coordinator := snapshot.NewRoomSnapshotCoordinator(snapshot.SessionFactoryFunc(func(snapshot.RoomSnapshotRequest, snapshot.SnapshotSink) (snapshot.Session, error) {
		created++
		return &snapshotSessionStub{id: "temporary-1"}, nil
	}), nil, func(payload string) (snapshot.Snapshot, error) { return snapshot.Parse(payload, false) }, time.Second)
	e, err := NewEngineWithOptions(model.MASTER, &config.Config{Channel: "source", Name: "bot"}, nil, EngineOptions{SnapshotCoordinator: coordinator})
	if err != nil {
		t.Fatal(err)
	}
	submitter, ok := interface{}(e).(common.RoomSnapshotSubmitter)
	if !ok {
		t.Fatalf("master engine lacks room snapshot capability: %T", e)
	}
	req := snapshot.RoomSnapshotRequest{WorkflowID: "wf-1", Author: "admin", SourceChannel: "source", TargetChannel: "target", Operation: snapshot.OperationFunc(func(snapshot.RoomSnapshotContext, snapshot.Snapshot) (snapshot.OperationResult, error) {
		return snapshot.Success(), nil
	})}
	if err := submitter.SubmitRoomSnapshot(req); err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatalf("coordinator sessions created = %d, want 1", created)
	}
}
