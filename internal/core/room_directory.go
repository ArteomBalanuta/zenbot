package core

import (
	"strings"

	"zenbot/internal/agent/tool/contract"
)

// RoomUserSnapshot is an immutable-at-return snapshot of a managed room's users.
type RoomUserSnapshot = contract.RoomUserSnapshot

// RoomUserDirectory provides read-only managed-room user snapshots.
type RoomUserDirectory interface {
	FindRoomUsers(room string) (RoomUserSnapshot, bool)
}

// EngineRoomUserDirectory reads the host and currently registered managed engines.
// It does not own engine or replica lifecycle.
type EngineRoomUserDirectory struct {
	Host     *EngineImpl
	Replicas *ReplicaManager
}

func (d EngineRoomUserDirectory) FindRoomUsers(room string) (RoomUserSnapshot, bool) {
	lookup := strings.TrimSpace(room)
	if lookup == "" || d.Host == nil {
		return RoomUserSnapshot{}, false
	}
	if strings.EqualFold(lookup, d.Host.GetChannel()) {
		return roomUserSnapshot(d.Host), true
	}
	if d.Replicas == nil {
		return RoomUserSnapshot{}, false
	}
	for _, engine := range d.Replicas.ManagedEngines() {
		impl, ok := engine.(*EngineImpl)
		if !ok || !strings.EqualFold(lookup, impl.GetChannel()) {
			continue
		}
		return roomUserSnapshot(impl), true
	}
	return RoomUserSnapshot{}, false
}

func roomUserSnapshot(engine *EngineImpl) RoomUserSnapshot {
	return RoomUserSnapshot{Room: engine.GetChannel(), Users: append([]string(nil), engine.ActiveUserNames()...)}
}
