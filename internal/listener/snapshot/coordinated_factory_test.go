package snapshot

import (
	"context"
	"errors"
	"testing"
)

func TestFactoryClosesPartiallyConstructedSessionAndRetainsCleanupError(t *testing.T) {
	registry := NewTemporarySessionRegistry()
	creationErr, closeErr := errors.New("construction failed"), errors.New("cleanup failed")
	s := &controlledSession{fakeSession: fakeSession{id: "partial"}, closeErr: closeErr, closeEntered: make(chan struct{}), closeRelease: make(chan struct{})}
	close(s.closeRelease)
	f := &CoordinatedSessionFactory{Registry: registry, New: func(context.Context, RoomSnapshotRequest, SnapshotSink) (Session, error) { return s, creationErr }}
	_, err := f.Create(RoomSnapshotRequest{}, nil)
	select {
	case <-s.closeEntered:
	default:
		t.Error("partial session was not closed")
	}
	if !errors.Is(err, creationErr) || !errors.Is(err, closeErr) || registry.Len() != 0 {
		t.Fatalf("error=%v registry=%d", err, registry.Len())
	}
}

type testSession struct {
	id              string
	closed, flushed int
}

func (s *testSession) ID() string           { return s.id }
func (s *testSession) Start() error         { return nil }
func (s *testSession) Close() error         { s.closed++; return nil }
func (s *testSession) Flush() error         { s.flushed++; return nil }
func (s *testSession) SendRaw(string) error { return nil }

func TestCoordinatedSessionFactoryCleansRegistryExactlyOnce(t *testing.T) {
	r := NewTemporarySessionRegistry()
	var made *testSession
	f := &CoordinatedSessionFactory{Registry: r, New: func(_ context.Context, _ RoomSnapshotRequest, _ SnapshotSink) (Session, error) {
		made = &testSession{id: "real"}
		return made, nil
	}}
	s, err := f.Create(RoomSnapshotRequest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Len() != 1 {
		t.Fatalf("registry len=%d", r.Len())
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if r.Len() != 0 || made.closed != 1 {
		t.Fatalf("len=%d closed=%d", r.Len(), made.closed)
	}
}
