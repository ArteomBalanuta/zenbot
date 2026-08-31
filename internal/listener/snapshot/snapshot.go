package snapshot

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"zenbot/internal/model"
)

type PayloadDecodeError struct{ Reason string }

func (e *PayloadDecodeError) Error() string { return "onlineSet: " + e.Reason }

type Snapshot struct {
	Users      []*model.User
	AgentShape bool
}

func Parse(payload string, agent bool) (Snapshot, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return Snapshot{}, &PayloadDecodeError{"malformed JSON"}
	}
	var cmd string
	if err := json.Unmarshal(root["cmd"], &cmd); err != nil || cmd != "onlineSet" {
		return Snapshot{}, &PayloadDecodeError{"cmd must be onlineSet"}
	}
	field := "users"
	if agent {
		field = "nicks"
	}
	raw, ok := root[field]
	if !ok {
		return Snapshot{}, &PayloadDecodeError{"missing " + field + " array"}
	}
	if agent {
		var nicks []string
		if err := json.Unmarshal(raw, &nicks); err != nil {
			return Snapshot{}, &PayloadDecodeError{"nicks must be strings"}
		}
		out := make([]*model.User, len(nicks))
		for i, n := range nicks {
			if n == "" {
				return Snapshot{}, &PayloadDecodeError{"blank nick"}
			}
			out[i] = &model.User{Name: n}
		}
		return Snapshot{Users: out, AgentShape: true}, nil
	}
	var users []*model.User
	if err := json.Unmarshal(raw, &users); err != nil {
		return Snapshot{}, &PayloadDecodeError{"users must be an array"}
	}
	for _, u := range users {
		if u == nil || u.Name == "" {
			return Snapshot{}, &PayloadDecodeError{"users must have a nonblank nick"}
		}
	}
	return Snapshot{Users: append([]*model.User(nil), users...)}, nil
}

type Store struct {
	mu    sync.RWMutex
	users map[string]*model.User
}

func NewStore() *Store { return &Store{users: map[string]*model.User{}} }
func (s *Store) Replace(users []*model.User) {
	next := map[string]*model.User{}
	for _, u := range users {
		next[model.IdentityKey(u.Trip, u.Hash, u.Name)] = u
	}
	s.mu.Lock()
	s.users = next
	s.mu.Unlock()
}
func (s *Store) Snapshot() []*model.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.User, 0, len(s.users))
	for _, u := range s.users {
		v := *u
		out = append(out, &v)
	}
	return out
}

type Operation func([]*model.User) error
type Coordinator struct {
	store   *Store
	timeout time.Duration
}

func NewCoordinator(store *Store, timeout time.Duration) *Coordinator {
	return &Coordinator{store: store, timeout: timeout}
}
func (c *Coordinator) Apply(payload string, agent bool) error {
	snap, err := Parse(payload, agent)
	if err != nil {
		return fmt.Errorf("snapshot decode: %w", err)
	}
	c.store.Replace(snap.Users)
	return nil
}
