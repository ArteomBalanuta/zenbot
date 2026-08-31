package core

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
)

type Replica interface{ Stop(context.Context) error }
type ReplicaManager struct {
	mu          sync.RWMutex
	replicas    map[string]Replica
	hostChannel string
	stopped     bool
}

func NewReplicaManager(hostChannel string) *ReplicaManager {
	return &ReplicaManager{replicas: map[string]Replica{}, hostChannel: hostChannel}
}
func (m *ReplicaManager) Add(channel string, r Replica) error {
	channel = strings.TrimSpace(channel)
	if channel == "" || r == nil {
		return errors.New("invalid replica")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return errors.New("replica manager is stopped")
	}
	if channel == m.hostChannel {
		return errors.New("cannot add host as replica")
	}
	if _, ok := m.replicas[channel]; ok {
		return errors.New("replica already exists")
	}
	m.replicas[channel] = r
	return nil
}
func (m *ReplicaManager) Remove(ctx context.Context, channel string) (Replica, error) {
	m.mu.Lock()
	r, ok := m.replicas[channel]
	if ok {
		delete(m.replicas, channel)
	}
	m.mu.Unlock()
	if !ok {
		return nil, errors.New("replica not found")
	}
	if err := r.Stop(ctx); err != nil {
		return r, err
	}
	return r, nil
}
func (m *ReplicaManager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	m.stopped = true
	rs := make([]Replica, 0, len(m.replicas))
	for _, r := range m.replicas {
		rs = append(rs, r)
	}
	m.replicas = map[string]Replica{}
	m.mu.Unlock()
	var first error
	for _, r := range rs {
		if err := r.Stop(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}
func (m *ReplicaManager) Replicas() map[string]Replica {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]Replica, len(m.replicas))
	for k, v := range m.replicas {
		out[k] = v
	}
	return out
}
func (m *ReplicaManager) Channels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.replicas))
	for k := range m.replicas {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
