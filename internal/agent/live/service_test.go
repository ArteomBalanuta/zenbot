package live

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/runtime"
)

func TestRuntimeServiceForwardsToOneRuntimeAndClosesIt(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	var mu sync.Mutex
	runs := 0
	runner := runtime.RunnerFunc(func(ctx context.Context, _ runtime.Invocation) (runtime.Result, error) {
		mu.Lock()
		runs++
		mu.Unlock()
		close(started)
		<-ctx.Done()
		close(cancelled)
		return runtime.Result{}, ctx.Err()
	})
	rt, err := runtime.New(runtime.Config{MaxConcurrent: 1, QueueCapacity: 0}, runner, nil)
	if err != nil {
		t.Fatal(err)
	}
	service := RuntimeService{Runtime: rt}
	invocation := runtimeServiceInvocation(t, "accepted")

	if err := service.Submit(invocation); err != nil {
		t.Fatalf("accepted submission: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("accepted invocation did not reach runtime runner")
	}
	mu.Lock()
	if runs != 1 {
		t.Fatalf("runtime runs=%d, want 1", runs)
	}
	mu.Unlock()

	full := make(chan error, 1)
	fullInvocation := runtimeServiceInvocation(t, "full")
	go func() { full <- service.Submit(fullInvocation) }()
	select {
	case err := <-full:
		if !errors.Is(err, runtime.ErrBusy) {
			t.Fatalf("full runtime submission error=%v, want ErrBusy", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("full runtime submission did not reject promptly")
	}

	service.Close()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("close did not cancel accepted runtime work")
	}
	if err := service.Submit(runtimeServiceInvocation(t, "late")); !errors.Is(err, runtime.ErrClosed) {
		t.Fatalf("late submission error=%v, want ErrClosed", err)
	}
}

func runtimeServiceInvocation(t *testing.T, requestID string) api.Invocation {
	t.Helper()
	ctx, err := api.NewContextWithCapabilities("room", "alice", "", "", false, []string{}, []api.Capability{})
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := api.NewInvocation(requestID, ctx, "question", api.DIRECT, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	return invocation
}
