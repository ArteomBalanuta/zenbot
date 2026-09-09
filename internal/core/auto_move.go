package core

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"zenbot/internal/common"
)

// AutoMoveState is shared by the host and all permanent replicas in this process.
type AutoMoveState struct {
	mu          sync.RWMutex
	enabled     bool
	sources     map[string]struct{}
	destination string
}

func NewAutoMoveState() *AutoMoveState {
	return &AutoMoveState{sources: map[string]struct{}{"purgatory": {}}, destination: "lounge"}
}
func (s *AutoMoveState) Snapshot() common.AutoMoveSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

func (s *AutoMoveState) Configure(source, destination string) (common.AutoMoveSnapshot, error) {
	source = strings.TrimSpace(source)
	destination = strings.TrimSpace(destination)
	if source == "" || destination == "" {
		return common.AutoMoveSnapshot{}, fmt.Errorf("automove source and destination must not be blank")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sources[source] = struct{}{}
	s.destination = destination
	return s.snapshotLocked(), nil
}

func (s *AutoMoveState) SetEnabled(enabled bool) common.AutoMoveSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
	return s.snapshotLocked()
}

func (s *AutoMoveState) snapshotLocked() common.AutoMoveSnapshot {
	out := common.AutoMoveSnapshot{Enabled: s.enabled, Destination: s.destination, Sources: make([]string, 0, len(s.sources))}
	for source := range s.sources {
		out.Sources = append(out.Sources, source)
	}
	sort.Strings(out.Sources)
	return out
}
func (s *AutoMoveState) EligibleReplica(channel string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.sources[channel]
	return s.destination, s.enabled && ok
}
