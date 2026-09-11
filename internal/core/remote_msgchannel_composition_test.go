package core

import (
	"testing"
	"time"

	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
)

type credentialedSnapshotSubmitter interface {
	SubmitCredentialedRoomSnapshot(snapshot.RoomSnapshotRequest) error
}

func TestMasterExposesCredentialedRemoteMsgChannelCapability(t *testing.T) {
	master := &EngineImpl{Type: model.MASTER, Password: "test-password"}
	if _, ok := BindCredentialedRoomSnapshotMaster(master).(credentialedSnapshotSubmitter); ok {
		t.Fatal("unconfigured master advertised credentialed snapshot submission")
	}
	coordinator := snapshot.NewRoomSnapshotCoordinator(snapshot.SessionFactoryFunc(func(snapshot.RoomSnapshotRequest, snapshot.SnapshotSink) (snapshot.Session, error) {
		return &roomSnapshotSessionStub{id: "configured-master"}, nil
	}), nil, func(payload string) (snapshot.Snapshot, error) { return snapshot.Parse(payload, false) }, time.Second)
	master.InstallRoomSnapshotCoordinator(coordinator)
	bound := BindCredentialedRoomSnapshotMaster(master)
	if _, ok := any(bound).(credentialedSnapshotSubmitter); !ok {
		t.Fatal("master does not expose credentialed snapshot submission")
	}
	if _, ok := any(master).(credentialedSnapshotSubmitter); ok {
		t.Fatal("unbound engine exposed master credential capability")
	}
	if _, ok := any(&EngineImpl{Type: model.REPLICA}).(credentialedSnapshotSubmitter); ok {
		t.Fatal("replica exposed master credential capability")
	}
	replica := &EngineImpl{Type: model.REPLICA}
	replica.InstallRoomSnapshotCoordinator(coordinator)
	if _, ok := BindCredentialedRoomSnapshotMaster(replica).(credentialedSnapshotSubmitter); ok {
		t.Fatal("configured replica exposed master credential capability")
	}
	if _, ok := any(&roomSnapshotSessionStub{}).(credentialedSnapshotSubmitter); ok {
		t.Fatal("temporary session exposed master credential capability")
	}
}
