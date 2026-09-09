package common

import "zenbot/internal/listener/snapshot"

// RoomSnapshotSubmitter submits a command-owned room snapshot workflow.
type RoomSnapshotSubmitter interface {
	SubmitRoomSnapshot(snapshot.RoomSnapshotRequest) error
}

// CredentialedRoomSnapshotSubmitter is master-owned and supplies credentials
// only to a temporary snapshot join; it never exposes the password.
type CredentialedRoomSnapshotSubmitter interface {
	SubmitCredentialedRoomSnapshot(snapshot.RoomSnapshotRequest) error
}
