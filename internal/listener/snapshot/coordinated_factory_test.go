package snapshot

import (
	"context"
	"testing"
)

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
