package core

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
)

type testReplica struct{ stops atomic.Int32 }

func (r *testReplica) Stop(context.Context) error { r.stops.Add(1); return nil }
func TestReplicaManagerOwnershipAndSnapshot(t *testing.T) {
	m := NewReplicaManager("main")
	r := &testReplica{}
	if m.Add("main", r) == nil {
		t.Fatal("host accepted")
	}
	if err := m.Add("room", r); err != nil {
		t.Fatal(err)
	}
	if m.Add("room", r) == nil {
		t.Fatal("duplicate accepted")
	}
	cp := m.Replicas()
	delete(cp, "room")
	if len(m.Replicas()) != 1 {
		t.Fatal("map leaked")
	}
	if err := m.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.stops.Load() != 1 {
		t.Fatal("stop count")
	}
}

func TestReplicaManagerRejectsBlankChannelsAndStopsNewAdds(t *testing.T) {
	m := NewReplicaManager("main")
	r := &testReplica{}
	if m.Add("  ", r) == nil {
		t.Fatal("blank channel accepted")
	}
	if err := m.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.Add("later", r); err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("error=%v", err)
	}
}
