package core

import (
	"testing"

	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
)

type credentialedSnapshotSubmitter interface {
	SubmitCredentialedRoomSnapshot(snapshot.RoomSnapshotRequest) error
}

func TestMasterExposesCredentialedRemoteMsgChannelCapability(t *testing.T) {
	master := &EngineImpl{Type: model.MASTER, Password: "test-password"}
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
	if _, ok := any(&roomSnapshotSessionStub{}).(credentialedSnapshotSubmitter); ok {
		t.Fatal("temporary session exposed master credential capability")
	}
}
