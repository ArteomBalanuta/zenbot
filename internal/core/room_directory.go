package core

import (
	"strings"
	"sync"

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
	// Host is retained for compatibility with statically composed directories.
	// New production composition uses binding so a persistent directory follows
	// each fresh master generation.
	Host     *EngineImpl
	Replicas *ReplicaManager
	binding  *roomDirectoryHostBinding
}

type roomDirectoryHostBinding struct {
	mu   sync.RWMutex
	host *EngineImpl
}

func NewEngineRoomUserDirectory(host *EngineImpl, replicas *ReplicaManager) *EngineRoomUserDirectory {
	return &EngineRoomUserDirectory{Host: host, Replicas: replicas, binding: &roomDirectoryHostBinding{host: host}}
}

func (d *EngineRoomUserDirectory) RebindHost(next *EngineImpl) {
	if d == nil {
		return
	}
	if d.binding == nil {
		d.binding = &roomDirectoryHostBinding{host: d.Host}
	}
	d.binding.mu.Lock()
	d.binding.host = next
	d.binding.mu.Unlock()
}

func (d EngineRoomUserDirectory) currentHost() *EngineImpl {
	if d.binding == nil {
		return d.Host
	}
	d.binding.mu.RLock()
	defer d.binding.mu.RUnlock()
	return d.binding.host
}

func (d EngineRoomUserDirectory) FindRoomUsers(room string) (RoomUserSnapshot, bool) {
	lookup := strings.TrimSpace(room)
	host := d.currentHost()
	if lookup == "" {
		return RoomUserSnapshot{}, false
	}
	if host != nil && strings.EqualFold(lookup, host.GetChannel()) {
		return roomUserSnapshot(host), true
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
