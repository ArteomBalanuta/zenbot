package runtime

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestRuntimeMemoryKeySeparatesPublicAndWhisperSessions(t *testing.T) {
	public := NewContext("room", "alice", "trip", "hash", false, nil)
	whisper := NewContext("room", "alice", "trip", "hash", true, nil)
	if public.MemoryKey() == whisper.MemoryKey() {
		t.Fatal("public and whisper sessions share a memory key")
	}
}

func TestInvocationAndContextAreImmutableContracts(t *testing.T) {
	users := []string{"alice"}
	ctx := NewContext("room", "alice", "trip", "hash", false, users)
	users[0] = "changed"
	inv := NewInvocation("request", ctx, "prompt", DIRECT, "message", true)

	if got := inv.Context().RoomUsers()[0]; got != "alice" {
		t.Fatalf("room users were not copied: %q", got)
	}
	if !inv.CommandOriginated() || inv.Mode() != DIRECT || inv.Prompt() != "prompt" {
		t.Fatalf("invocation contract lost fields: %+v", inv)
	}
	if got := NewResult("correlation", "answer", false); got.ShouldReply() {
		t.Fatal("no-reply result should not require delivery")
	}
}

func TestRuntimeRejectsWhenAdmissionQueueIsFull(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	runner := RunnerFunc(func(ctx context.Context, inv Invocation) (Result, error) {
		select {
		case <-started:
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
			close(started)
		}
		select {
		case <-release:
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
		return NewResult(inv.RequestID(), "ok", true), nil
	})
	rt := mustRuntime(t, Config{MaxConcurrent: 1, QueueCapacity: 1}, runner, nil)
	defer rt.Close()

	if err := rt.Submit(invocation("one", "r1")); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	<-started
	if err := rt.Submit(invocation("two", "r2")); err != nil {
		t.Fatalf("queued submit: %v", err)
	}
	if err := rt.Submit(invocation("three", "r3")); !errors.Is(err, ErrBusy) {
		t.Fatalf("third submit error = %v, want ErrBusy", err)
	}
	close(release)
}

func TestRuntimeSerializesInvocationsPerRoomButNotAcrossRooms(t *testing.T) {
	var mu sync.Mutex
	active := map[string]int{}
	maxActive := map[string]int{}
	release := make(chan struct{})
	runner := RunnerFunc(func(ctx context.Context, inv Invocation) (Result, error) {
		room := inv.Context().Room()
		mu.Lock()
		active[room]++
		if active[room] > maxActive[room] {
			maxActive[room] = active[room]
		}
		mu.Unlock()
		select {
		case <-release:
		case <-ctx.Done():
		}
		mu.Lock()
		active[room]--
		mu.Unlock()
		return NewResult(inv.RequestID(), room, false), nil
	})
	rt := mustRuntime(t, Config{MaxConcurrent: 2, QueueCapacity: 4}, runner, nil)
	defer rt.Close()
	for _, item := range []struct{ id, room string }{{"a", "same"}, {"b", "same"}, {"c", "other"}} {
		if err := rt.Submit(invocation(item.id, item.room)); err != nil {
			t.Fatalf("submit %s: %v", item.id, err)
		}
	}
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return maxActive["same"] == 1 && maxActive["other"] == 1
	})
	close(release)
}

func TestRuntimeDeliversRepliesAndHonorsCancellationAndShutdown(t *testing.T) {
	var got []string
	var mu sync.Mutex
	sink := SinkFunc(func(ctx context.Context, inv Invocation, result Result) error {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, inv.RequestID()+":"+result.Text())
		return nil
	})
	cancelled := make(chan struct{})
	cancelStarted := make(chan struct{})
	runner := RunnerFunc(func(ctx context.Context, inv Invocation) (Result, error) {
		if inv.RequestID() == "cancel" {
			close(cancelStarted)
			<-ctx.Done()
			close(cancelled)
			return Result{}, ctx.Err()
		}
		return NewResult(inv.RequestID(), "answer", true), nil
	})
	rt := mustRuntime(t, Config{MaxConcurrent: 1, QueueCapacity: 1}, runner, sink)
	if err := rt.Submit(invocation("answer", "room")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return reflect.DeepEqual(got, []string{"answer:answer"}) })
	if err := rt.Submit(invocation("cancel", "room2")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelStarted:
	case <-time.After(time.Second):
		t.Fatal("cancel test runner did not start")
	}
	rt.Close()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("runner was not cancelled during shutdown")
	}
	if err := rt.Submit(invocation("after", "room")); !errors.Is(err, ErrClosed) {
		t.Fatalf("submit after close = %v, want ErrClosed", err)
	}
}

func mustRuntime(t *testing.T, cfg Config, runner Runner, sink Sink) *Runtime {
	t.Helper()
	rt, err := New(cfg, runner, sink)
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func invocation(id, room string) Invocation {
	return NewInvocation(id, NewContext(room, "alice", "trip", "hash", false, nil), id, DIRECT, "", false)
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met")
}
