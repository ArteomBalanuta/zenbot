package runtime

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRuntimeLogsFailedInvocationWithCorrelationAndCause(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	failureDelivered := make(chan struct{})
	rt, err := NewWithFailureSink(
		Config{MaxConcurrent: 1, QueueCapacity: 1},
		RunnerFunc(func(context.Context, Invocation) (Result, error) {
			return Result{}, errors.New("provider timed out")
		}),
		nil,
		FailureSinkFunc(func(context.Context, Invocation, error) { close(failureDelivered) }),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if err := rt.Submit(NewInvocation("request-99", NewContext("programming", "alice", "", "", false, nil), "hello", DIRECT, "", true)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-failureDelivered:
	case <-time.After(time.Second):
		t.Fatal("failure was not delivered")
	}

	logged := output.String()
	for _, expected := range []string{"agent.request.started", "agent.request.failed", "request_id=request-99", "mode=DIRECT", "room=programming", "nick=alice", "error=\"provider timed out\""} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log %q does not contain %q", logged, expected)
		}
	}
}

func TestRuntimeFailureSinkOnlyRepliesForRequiredModes(t *testing.T) {
	var got []Mode
	var mu sync.Mutex
	failure := FailureSinkFunc(func(_ context.Context, inv Invocation, _ error) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, inv.Mode())
	})
	rt, err := NewWithFailureSink(Config{MaxConcurrent: 1, QueueCapacity: 1}, RunnerFunc(func(_ context.Context, inv Invocation) (Result, error) { return Result{}, errors.New("failed") }), nil, failure)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if err := rt.Submit(NewInvocation("mention", NewContext("r", "n", "", "", false, nil), "p", MENTION, "", false)); err != nil {
		t.Fatal(err)
	}
	if err := rt.Submit(NewInvocation("ambient", NewContext("r2", "n", "", "", false, nil), "p", AMBIENT, "", false)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(got) == 1 })
	mu.Lock()
	defer mu.Unlock()
	if got[0] != MENTION {
		t.Fatalf("failures=%v", got)
	}
}

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

type postDeliveryRunner struct {
	result Result
	mu     sync.Mutex
	calls  int
}

func (r *postDeliveryRunner) Run(context.Context, Invocation) (Result, error) { return r.result, nil }
func (r *postDeliveryRunner) AfterDelivery(context.Context, Invocation, Result) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return nil
}
func (r *postDeliveryRunner) persisted() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestRuntimeCallsPostDeliveryOnlyAfterSuccessfulVisibleSink(t *testing.T) {
	for _, tc := range []struct {
		name       string
		result     Result
		sinkErr    error
		wantWrites int
	}{
		{name: "successful visible delivery", result: NewResult("success", "visible", true), wantWrites: 1},
		{name: "sink failure", result: NewResult("sink-failure", "visible", true), sinkErr: errors.New("sink failure"), wantWrites: 0},
		{name: "no reply", result: NewResult("no-reply", "", false), wantWrites: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &postDeliveryRunner{result: tc.result}
			rt := mustRuntime(t, Config{MaxConcurrent: 1, QueueCapacity: 1}, runner, SinkFunc(func(context.Context, Invocation, Result) error { return tc.sinkErr }))
			defer rt.Close()
			if err := rt.Submit(invocation(tc.result.CorrelationID(), "room")); err != nil {
				t.Fatal(err)
			}
			if tc.wantWrites == 1 {
				waitFor(t, func() bool { return runner.persisted() == tc.wantWrites })
			} else {
				time.Sleep(20 * time.Millisecond)
				if got := runner.persisted(); got != tc.wantWrites {
					t.Fatalf("post-delivery writes=%d, want %d", got, tc.wantWrites)
				}
			}
		})
	}
}

func TestRuntimePersistsConfirmedToolOwnedDeliveryWithoutSecondSinkMessage(t *testing.T) {
	runner := &postDeliveryRunner{result: NewToolOwnedResult("tool-owned", "Completed the requested action.", nil)}
	var sinkCalls atomic.Int32
	rt := mustRuntime(t, Config{MaxConcurrent: 1, QueueCapacity: 1}, runner, SinkFunc(func(context.Context, Invocation, Result) error {
		sinkCalls.Add(1)
		return nil
	}))
	defer rt.Close()
	if err := rt.Submit(invocation("tool-owned", "room")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return runner.persisted() == 1 })
	if sinkCalls.Load() != 0 {
		t.Fatalf("tool-owned delivery emitted %d duplicate sink messages", sinkCalls.Load())
	}
}

func TestRuntimeAppliesRequestWideDeadline(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan error, 1)
	rt := mustRuntime(t, Config{MaxConcurrent: 1, QueueCapacity: 1, RequestTimeout: 15 * time.Millisecond}, RunnerFunc(func(ctx context.Context, _ Invocation) (Result, error) {
		close(started)
		<-ctx.Done()
		cancelled <- ctx.Err()
		return Result{}, ctx.Err()
	}), nil)
	defer rt.Close()
	if err := rt.Submit(invocation("deadline", "room")); err != nil {
		t.Fatal(err)
	}
	<-started
	select {
	case err := <-cancelled:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("runner cancellation=%v, want deadline exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request deadline did not cancel runner")
	}
}
