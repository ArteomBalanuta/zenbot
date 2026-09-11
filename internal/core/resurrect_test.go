package core

import (
	"context"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

func TestMoveFromServingRoomPrefersExactManagedReplicaThenHost(t *testing.T) {
	host := &EngineImpl{Channel: "host", ActiveUsers: map[*model.User]struct{}{&model.User{Name: "Alice"}: {}}, OutMessageQueue: make(chan string, 1)}
	replica := &EngineImpl{Channel: "source", ActiveUsers: map[*model.User]struct{}{&model.User{Name: "@Alice"}: {}}, OutMessageQueue: make(chan string, 1)}
	manager := NewReplicaManager(host.Channel)
	if err := manager.Add("source", managedReplica{ManagedEngine: replica}); err != nil {
		t.Fatal(err)
	}
	host.SetReplicaController(NewManagedReplicaController(manager, nil))

	handled, err := host.MoveFromServingRoom(context.Background(), "source", "@alice", common.Channel("destination"))
	if !handled || err != nil {
		t.Fatalf("handled=%t err=%v", handled, err)
	}
	if got := <-replica.OutMessageQueue; got != `{"cmd":"kick","nick":"@Alice","to":"destination"}` {
		t.Fatalf("replica payload=%q", got)
	}
	if len(host.OutMessageQueue) != 0 {
		t.Fatal("host handled replica source")
	}

	handled, err = host.MoveFromServingRoom(context.Background(), "host", "alice", common.Channel("destination"))
	if !handled || err != nil {
		t.Fatalf("host handled=%t err=%v", handled, err)
	}
	if got := <-host.OutMessageQueue; got != `{"cmd":"kick","nick":"Alice","to":"destination"}` {
		t.Fatalf("host payload=%q", got)
	}
}

func TestMoveFromServingRoomDoesNotActForMissingOrCancelledSource(t *testing.T) {
	host := &EngineImpl{Channel: "host", OutMessageQueue: make(chan string, 1)}
	if handled, err := host.MoveFromServingRoom(context.Background(), "missing", "Alice", common.Channel("destination")); handled || err != nil {
		t.Fatalf("missing handled=%t err=%v", handled, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if handled, err := host.MoveFromServingRoom(ctx, "host", "Alice", common.Channel("destination")); handled || err != context.Canceled {
		t.Fatalf("cancelled handled=%t err=%v", handled, err)
	}
	if len(host.OutMessageQueue) != 0 {
		t.Fatal("missing or cancelled move emitted output")
	}
}
