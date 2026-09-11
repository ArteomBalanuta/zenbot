package core

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"zenbot/internal/common"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
)

// InstallRoomSnapshotCoordinator installs the lifecycle-owning coordinator at
// engine construction time.
func (e *EngineImpl) InstallRoomSnapshotCoordinator(coordinator *snapshot.RoomSnapshotCoordinator) {
	e.snapshotCoordinator = coordinator
}

// SubmitRoomSnapshot forwards legacy credential-free requests unchanged.
func (e *EngineImpl) SubmitRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	if e.snapshotCoordinator == nil {
		return errors.New("room snapshot coordinator is not configured")
	}
	return e.snapshotCoordinator.Submit(request)
}

// credentialedRoomSnapshotMaster is the sole engine view that can submit a
// credentialed temporary room join. It is bound only at master command
// composition and deliberately not implemented by EngineImpl itself.
type credentialedRoomSnapshotMaster struct{ *EngineImpl }

func (e *credentialedRoomSnapshotMaster) RegisterCommand(command common.Command) {
	e.EngineImpl.registerCommandFor(command, e)
}

// BindCredentialedRoomSnapshotMaster returns a command-facing engine view that
// can supply a host password exclusively to a temporary snapshot join. Only a
// master with its construction-time coordinator installed exposes this
// capability; other engines retain their ordinary engine view.
func BindCredentialedRoomSnapshotMaster(engine *EngineImpl) common.Engine {
	if engine == nil || engine.Type != model.MASTER || engine.snapshotCoordinator == nil {
		return engine
	}
	return &credentialedRoomSnapshotMaster{EngineImpl: engine}
}

// SubmitCredentialedRoomSnapshot supplies the master password only to the
// temporary join descriptor used by a privileged remote-room workflow.
func (e *credentialedRoomSnapshotMaster) SubmitCredentialedRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	if e == nil || e.EngineImpl == nil || e.snapshotCoordinator == nil {
		return errors.New("room snapshot coordinator is not configured")
	}
	if len(request.WorkflowID) < 8 || request.TargetChannel == "" {
		return errors.New("credentialed room snapshot request is invalid")
	}
	request.TemporaryJoin = &snapshot.TemporaryJoin{Channel: request.TargetChannel, Nick: temporarySnapshotNick(request.WorkflowID), Password: e.Password}
	return e.snapshotCoordinator.Submit(request)
}

func temporarySnapshotNick(workflowID string) string {
	digest := sha256.Sum256([]byte(workflowID))
	return fmt.Sprintf("msg_%x", digest[:8])
}
