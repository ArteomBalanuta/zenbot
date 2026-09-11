package main

import (
	"errors"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/factory"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/profiling"
)

func TestRoomSnapshotReplySinkReturnsDeliveryFailure(t *testing.T) {
	want := errors.New("delivery failed")
	sink := roomSnapshotReplySink(func(string, string, bool) (string, error) { return "", want })
	if err := sink(snapshot.RoomSnapshotRequest{Author: "alice"}, "reply"); !errors.Is(err, want) {
		t.Fatalf("error=%v", err)
	}
}

func TestMasterBindingSnapshotReplyUsesReboundMaster(t *testing.T) {
	old := &core.EngineImpl{OutMessageQueue: make(chan string, 1)}
	next := &core.EngineImpl{OutMessageQueue: make(chan string, 1)}
	binding := newMasterBinding(old)
	sink := roomSnapshotReplySink(masterReplySender(binding))

	binding.Rebind(next)
	sink(snapshot.RoomSnapshotRequest{Author: "alice"}, "reply")

	select {
	case <-old.OutMessageQueue:
		t.Fatal("snapshot reply was sent through the retired master")
	default:
	}
	select {
	case <-next.OutMessageQueue:
	default:
		t.Fatal("snapshot reply was not sent through the rebound master")
	}
}

func TestMasterBindingUnbindRejectsReply(t *testing.T) {
	old := &core.EngineImpl{OutMessageQueue: make(chan string, 1)}
	binding := newMasterBinding(old)
	binding.Rebind(nil)
	if _, err := masterReplySender(binding)("alice", "reply", false); err == nil {
		t.Fatal("reply used unavailable host")
	}
	if len(old.OutMessageQueue) != 0 {
		t.Fatal("retired host received reply")
	}
}

func TestRoomSnapshotReplySinkPreservesRequestWhisperMode(t *testing.T) {
	type delivery struct {
		author  string
		reply   string
		whisper bool
	}
	var deliveries []delivery
	sink := roomSnapshotReplySink(func(author, reply string, whisper bool) (string, error) {
		deliveries = append(deliveries, delivery{author: author, reply: reply, whisper: whisper})
		return "", nil
	})

	sink(snapshot.RoomSnapshotRequest{Author: "whisper-author", Whisper: true}, "private")
	sink(snapshot.RoomSnapshotRequest{Author: "public-author", Whisper: false}, "public")

	if len(deliveries) != 2 || deliveries[0] != (delivery{"whisper-author", "private", true}) || deliveries[1] != (delivery{"public-author", "public", false}) {
		t.Fatalf("deliveries = %#v, want exact author/reply/whisper delivery", deliveries)
	}
}

func TestRoomSnapshotEngineOptionsInstallsCoordinatorOnMaster(t *testing.T) {
	cfg := &config.Config{Channel: "source", Name: "bot", WebsocketUrl: "ws://example.test"}
	profiler := profiling.New(profiling.Settings{Enabled: true}, nil)
	opts := newRoomSnapshotEngineOptions(cfg, nil, func(snapshot.RoomSnapshotRequest, string) error { return nil }, profiler)
	if opts.SessionRegistry == nil || opts.SnapshotCoordinator == nil {
		t.Fatalf("snapshot options = %#v, want registry and coordinator", opts)
	}
	if opts.Profiler != profiler || opts.Transport.Profiler != profiler {
		t.Fatal("snapshot engine options did not propagate the process profiler")
	}

	engine, err := factory.NewEngineWithOptions(model.MASTER, cfg, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := any(engine).(common.RoomSnapshotSubmitter); !ok {
		t.Fatal("master is missing room snapshot submitter")
	}
	if _, ok := any(engine).(common.LiveRoomMover); !ok {
		t.Fatal("master is missing live room mover")
	}
	if engine.PerformanceProfiler() != profiler {
		t.Fatal("master engine did not retain the process profiler")
	}
	request := snapshot.RoomSnapshotRequest{WorkflowID: "workflow", Author: "author", SourceChannel: "source", TargetChannel: "target"}
	if err := engine.SubmitRoomSnapshot(request); err == nil || err.Error() != "operation cannot be nil" {
		t.Fatalf("SubmitRoomSnapshot() error = %v, want coordinator validation error", err)
	}
	if got := opts.SessionRegistry.Len(); got != 0 {
		t.Fatalf("temporary registry length = %d, want 0", got)
	}
}
