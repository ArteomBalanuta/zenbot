package core

import (
	"testing"
	"time"

	"zenbot/internal/common"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
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

type captureRegistrationCommand struct {
	captured *common.Engine
	role     model.Role
}

func (c *captureRegistrationCommand) Execute()             {}
func (c *captureRegistrationCommand) GetRole() *model.Role { return &c.role }
func (c *captureRegistrationCommand) GetAliases() []string { return []string{"capture"} }
func (c *captureRegistrationCommand) NewInstance(engine common.Engine, _ *model.ChatMessage) common.Command {
	*c.captured = engine
	return c
}

func TestCredentialedMasterPreservesWrapperCapabilitiesWhenRegisteringCommands(t *testing.T) {
	engine := &EngineImpl{Type: model.MASTER, EnabledCommands: map[string]common.CommandMetadata{}}
	bound := BindCredentialedRoomSnapshotMaster(engine)
	var captured common.Engine
	bound.RegisterCommand(&captureRegistrationCommand{captured: &captured})
	(*bound.GetEnabledCommands())["capture"].Command(&model.ChatMessage{})

	if _, ok := captured.(common.CredentialedRoomSnapshotSubmitter); !ok {
		t.Fatalf("registered command engine %T lost credentialed snapshot capability", captured)
	}
}

func TestCredentialedRoomSnapshotUsesProtocolValidTemporaryNick(t *testing.T) {
	var captured snapshot.RoomSnapshotRequest
	coordinator := snapshot.NewRoomSnapshotCoordinator(snapshot.SessionFactoryFunc(func(request snapshot.RoomSnapshotRequest, _ snapshot.SnapshotSink) (snapshot.Session, error) {
		captured = request
		return &roomSnapshotSessionStub{id: "temporary-list"}, nil
	}), nil, func(payload string) (snapshot.Snapshot, error) { return snapshot.Parse(payload, false) }, time.Minute)
	engine := &EngineImpl{Type: model.MASTER, Password: "secret"}
	engine.InstallRoomSnapshotCoordinator(coordinator)
	submitter := BindCredentialedRoomSnapshotMaster(engine).(common.CredentialedRoomSnapshotSubmitter)
	request := validRoomSnapshotRequest()
	request.WorkflowID = "12345678-abcd"
	request.TargetChannel = "lounge"

	if err := submitter.SubmitCredentialedRoomSnapshot(request); err != nil {
		t.Fatal(err)
	}
	defer coordinator.Cancel(request.WorkflowID, "test complete")

	if captured.TemporaryJoin == nil {
		t.Fatal("credentialed snapshot did not receive a temporary join")
	}
	if captured.TemporaryJoin.Channel != "lounge" || captured.TemporaryJoin.Nick != "msg_12345678" || captured.TemporaryJoin.Password != "secret" {
		t.Fatalf("temporary join = %#v", captured.TemporaryJoin)
	}
}
