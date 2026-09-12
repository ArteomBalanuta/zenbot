package command

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
)

type listCompletionEngine struct {
	commandEngineStub
	request chan snapshot.RoomSnapshotRequest
}

func TestListCancellationDoesNotBlockLateCompletion(t *testing.T) {
	e := &listCompletionEngine{request: make(chan snapshot.RoomSnapshotRequest, 1)}
	definition, _ := commandDefinitionFor("list")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan error, 1)
	go func() {
		_, err := definition.New(e, &model.ChatMessage{Name: "alice", Text: "!list remote"}).Execute(ctx)
		returned <- err
	}()
	request := <-e.request
	cancel()
	select {
	case err := <-returned:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not release command")
	}
	completed := make(chan struct{})
	go func() { request.OnComplete(snapshot.Failed("cancelled")); close(completed) }()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("late completion blocked workflow cleanup")
	}
}

type delayedListSession struct{ started chan struct{} }

func (*delayedListSession) ID() string           { return "delayed-list" }
func (s *delayedListSession) Start() error       { close(s.started); return nil }
func (*delayedListSession) Close() error         { return nil }
func (*delayedListSession) Flush() error         { return nil }
func (*delayedListSession) SendRaw(string) error { return nil }

func TestListKeepsCoordinatorAliveUntilDelayedRosterDelivery(t *testing.T) {
	started := make(chan struct{})
	sinks := make(chan snapshot.SnapshotSink, 1)
	replies := make(chan string, 1)
	coordinator := snapshot.NewRoomSnapshotCoordinator(snapshot.SessionFactoryFunc(func(_ snapshot.RoomSnapshotRequest, sink snapshot.SnapshotSink) (snapshot.Session, error) {
		sinks <- sink
		return &delayedListSession{started: started}, nil
	}), func(_ snapshot.RoomSnapshotRequest, reply string) error { replies <- reply; return nil }, func(raw string) (snapshot.Snapshot, error) { return snapshot.Parse(raw, false) }, time.Second)
	e := &ownedSnapshotEngine{gatewayEngine: &gatewayEngine{}, coordinator: coordinator}
	definition, _ := commandDefinitionFor("list")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan model.Status, 1)
	go func() {
		defer cancel()
		status, _ := definition.New(e, &model.ChatMessage{Name: "alice", Text: "!list remote"}).Execute(ctx)
		returned <- status
	}()
	<-started
	select {
	case <-returned:
		t.Fatal("command returned before roster arrival")
	case <-time.After(20 * time.Millisecond):
	}
	sink := <-sinks
	sink(`{"cmd":"onlineSet","users":[{"nick":"bob","trip":"trip"}]}`)
	select {
	case status := <-returned:
		if status != model.SUCCESSFUL {
			t.Fatalf("status=%v", status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("snapshot did not complete")
	}
	if reply := <-replies; !strings.Contains(reply, "bob") || strings.Contains(reply, "Unable") {
		t.Fatalf("reply=%q", reply)
	}
}

func (e *listCompletionEngine) SubmitCredentialedRoomSnapshot(r snapshot.RoomSnapshotRequest) error {
	e.request <- r
	return nil
}

func TestListWaitsForTerminalSnapshot(t *testing.T) {
	for _, failed := range []bool{false, true} {
		e := &listCompletionEngine{request: make(chan snapshot.RoomSnapshotRequest, 1)}
		definition, _ := commandDefinitionFor("list")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		returned := make(chan model.Status, 1)
		go func() {
			defer cancel()
			status, _ := definition.New(e, &model.ChatMessage{Name: "alice", Text: "!list remote"}).Execute(ctx)
			returned <- status
		}()
		request := <-e.request
		if request.OnComplete == nil {
			<-returned
			t.Fatal("list does not await a terminal snapshot result")
		}
		select {
		case <-returned:
			t.Fatal("list returned before snapshot completed")
		case <-time.After(20 * time.Millisecond):
		}
		if ctx.Err() != nil {
			t.Fatal("request lifetime ended before snapshot")
		}
		result := snapshot.Success()
		if failed {
			result = snapshot.Failed("remote room failure")
		}
		request.OnComplete(result)
		select {
		case status := <-returned:
			if (status == model.SUCCESSFUL) == failed {
				t.Fatalf("failed=%v status=%v", failed, status)
			}
		case <-time.After(time.Second):
			t.Fatal("list did not finish")
		}
	}
}
