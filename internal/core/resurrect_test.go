package core

import (
	"context"
	"testing"

	"zenbot/internal/common"
)

func TestMoveFromServingRoomPrefersExactManagedReplicaThenHost(t *testing.T) {
	host := &EngineImpl{Channel: "host", OutMessageQueue: make(chan string, 1)}
	replica := &EngineImpl{Channel: "source", OutMessageQueue: make(chan string, 1)}
	manager := NewReplicaManager(host.Channel)
	if err := manager.Add("source", managedReplica{ManagedEngine: replica}); err != nil {
		t.Fatal(err)
	}
	host.SetReplicaController(NewManagedReplicaController(manager, nil))

	handled, err := host.MoveFromServingRoom(context.Background(), "source", common.NickTarget("@Alice"), common.Channel("destination"))
	if !handled || err != nil {
		t.Fatalf("handled=%t err=%v", handled, err)
	}
	if got := <-replica.OutMessageQueue; got != `{"cmd":"kick","nick":"@Alice","to":"destination"}` {
		t.Fatalf("replica payload=%q", got)
	}
	if len(host.OutMessageQueue) != 0 {
		t.Fatal("host handled replica source")
	}

	handled, err = host.MoveFromServingRoom(context.Background(), "host", common.NickTarget("Alice"), common.Channel("destination"))
	if !handled || err != nil {
		t.Fatalf("host handled=%t err=%v", handled, err)
	}
	if got := <-host.OutMessageQueue; got != `{"cmd":"kick","nick":"Alice","to":"destination"}` {
		t.Fatalf("host payload=%q", got)
	}
}

func TestMoveFromServingRoomDoesNotActForMissingOrCancelledSource(t *testing.T) {
	host := &EngineImpl{Channel: "host", OutMessageQueue: make(chan string, 1)}
	if handled, err := host.MoveFromServingRoom(context.Background(), "missing", common.NickTarget("Alice"), common.Channel("destination")); handled || err != nil {
		t.Fatalf("missing handled=%t err=%v", handled, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if handled, err := host.MoveFromServingRoom(ctx, "host", common.NickTarget("Alice"), common.Channel("destination")); handled || err != context.Canceled {
		t.Fatalf("cancelled handled=%t err=%v", handled, err)
	}
	if len(host.OutMessageQueue) != 0 {
		t.Fatal("missing or cancelled move emitted output")
	}
}
