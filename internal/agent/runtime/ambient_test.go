package runtime

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	agentapi "zenbot/internal/agent/api"
)

func TestRuntimeAmbientLatestWinsWithoutOrdinaryAdmission(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	var executed []string
	runner := RunnerFunc(func(ctx context.Context, inv Invocation) (Result, error) {
		mu.Lock()
		executed = append(executed, inv.RequestID())
		mu.Unlock()
		if inv.RequestID() == "first" {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		}
		return NewResult(inv.RequestID(), "ok", false), nil
	})
	rt := mustRuntime(t, Config{MaxConcurrent: 1, QueueCapacity: 0}, runner, nil)
	defer rt.Close()
	if err := rt.SubmitAmbient(ambientInvocation("first", "room")); err != nil {
		t.Fatalf("first ambient: %v", err)
	}
	<-started
	if err := rt.Submit(invocation("direct", "room")); err != nil {
		t.Fatalf("ordinary admission: %v", err)
	}
	if err := rt.SubmitAmbient(ambientInvocation("stale", "room")); err != nil {
		t.Fatalf("stale ambient: %v", err)
	}
	if err := rt.SubmitAmbient(ambientInvocation("latest", "room")); err != nil {
		t.Fatalf("latest ambient: %v", err)
	}
	close(release)
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(executed) == 3
	})
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(executed, []string{"first", "direct", "latest"}) {
		t.Fatalf("executed = %v", executed)
	}
}

func TestAPIBridgeRoutesAmbientPastFullOrdinaryAdmission(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	var executed []string
	rt := mustRuntime(t, Config{MaxConcurrent: 1, QueueCapacity: 0}, RunnerFunc(func(ctx context.Context, inv Invocation) (Result, error) {
		mu.Lock()
		executed = append(executed, inv.RequestID())
		mu.Unlock()
		if inv.RequestID() == "direct" {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		}
		return NewResult(inv.RequestID(), "ok", false), nil
	}), nil)
	defer rt.Close()

	if err := rt.Submit(invocation("direct", "room")); err != nil {
		t.Fatal(err)
	}
	<-started
	ctx, err := agentapi.NewContext("room", "alice", "trip", "hash", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	ambient, err := agentapi.NewInvocation("ambient", ctx, "ambient", agentapi.AMBIENT, "ambient", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := (APIBridge{Runtime: rt}).Submit(ambient); err != nil {
		t.Fatalf("ambient bridge submission while ordinary admission is full: %v", err)
	}
	close(release)
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return reflect.DeepEqual(executed, []string{"direct", "ambient"})
	})
}

func TestRuntimeCloseCancelsRunningAmbientAndDropsPending(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	var mu sync.Mutex
	var executed []string
	rt := mustRuntime(t, Config{MaxConcurrent: 1, QueueCapacity: 0}, RunnerFunc(func(ctx context.Context, inv Invocation) (Result, error) {
		mu.Lock()
		executed = append(executed, inv.RequestID())
		mu.Unlock()
		if inv.RequestID() == "running" {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return Result{}, ctx.Err()
		}
		return NewResult(inv.RequestID(), "ok", false), nil
	}), nil)
	if err := rt.SubmitAmbient(ambientInvocation("running", "room")); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := rt.SubmitAmbient(ambientInvocation("pending", "room")); err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	go func() { rt.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not wait for ambient execution")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("running ambient was not cancelled")
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(executed, []string{"running"}) {
		t.Fatalf("executed = %v", executed)
	}
	if err := rt.SubmitAmbient(ambientInvocation("after", "room")); !errors.Is(err, ErrClosed) {
		t.Fatalf("submit after close = %v", err)
	}
}

func ambientInvocation(id, room string) Invocation {
	return NewInvocation(id, NewContext(room, "alice", "trip", "hash", false, nil), id, AMBIENT, "", false)
}
