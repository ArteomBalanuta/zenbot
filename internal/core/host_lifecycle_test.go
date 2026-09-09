package core

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestHostLifecycleSerializesAndCoalescesRequests(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	var restarts, shutdowns int
	controller := NewHostLifecycle(func(context.Context) error {
		mu.Lock()
		restarts++
		mu.Unlock()
		close(started)
		<-release
		return nil
	}, func(context.Context) error {
		mu.Lock()
		shutdowns++
		mu.Unlock()
		return nil
	})
	defer controller.Close()

	if err := controller.RequestRestart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.RequestRestart(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("restart did not begin")
	}
	if err := controller.RequestRestart(context.Background()); err != nil {
		t.Fatal(err)
	}
	close(release)
	controller.Wait()
	mu.Lock()
	defer mu.Unlock()
	if restarts != 1 || shutdowns != 0 {
		t.Fatalf("restarts=%d shutdowns=%d", restarts, shutdowns)
	}
}

func TestHostLifecycleDefersRestartUntilSubmittingDispatchReleases(t *testing.T) {
	started := make(chan struct{})
	controller := NewHostLifecycle(func(context.Context) error {
		close(started)
		return nil
	}, nil)
	defer controller.Close()

	releaseDispatch := controller.BeginDispatch()
	if err := controller.RequestRestart(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
		t.Fatal("restart began before the submitting dispatch returned")
	case <-time.After(50 * time.Millisecond):
	}

	releaseDispatch()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("restart did not begin after dispatch release")
	}
	controller.Wait()
}

func TestHostLifecycleShutdownSupersedesRestartAndIsTerminal(t *testing.T) {
	allowRestart := make(chan struct{})
	started := make(chan struct{})
	var mu sync.Mutex
	var restarts, shutdowns int
	controller := NewHostLifecycle(func(context.Context) error {
		mu.Lock()
		restarts++
		mu.Unlock()
		close(started)
		<-allowRestart
		return nil
	}, func(context.Context) error { mu.Lock(); shutdowns++; mu.Unlock(); return nil })
	defer controller.Close()
	if err := controller.RequestRestart(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := controller.RequestShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.RequestRestart(context.Background()); err != nil {
		t.Fatal(err)
	}
	close(allowRestart)
	controller.Wait()
	mu.Lock()
	defer mu.Unlock()
	if restarts != 1 || shutdowns != 1 {
		t.Fatalf("restarts=%d shutdowns=%d", restarts, shutdowns)
	}
}

func TestHostLifecycleDoesNotAdmitCanceledRequest(t *testing.T) {
	var calls int
	controller := NewHostLifecycle(func(context.Context) error { calls++; return nil }, func(context.Context) error { calls++; return nil })
	defer controller.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := controller.RequestRestart(ctx); err != context.Canceled {
		t.Fatalf("err=%v", err)
	}
	controller.Wait()
	if calls != 0 {
		t.Fatalf("calls=%d", calls)
	}
}
