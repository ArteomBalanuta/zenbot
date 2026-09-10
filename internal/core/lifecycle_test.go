package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type lifecycleFake struct{ stopped atomic.Int32 }

func (f *lifecycleFake) Start(context.Context) error { return nil }
func (f *lifecycleFake) Stop(context.Context) error  { f.stopped.Add(1); return nil }
func (f *lifecycleFake) Healthy() bool               { return true }
func TestLifecycleStartStopRestartIsBounded(t *testing.T) {
	var made atomic.Int32
	var last *lifecycleFake
	l := NewLifecycle(func() LifecycleEngine { made.Add(1); last = &lifecycleFake{}; return last }, RetryPolicy{HealthInterval: time.Millisecond, StopTimeout: time.Second})
	if err := l.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for !l.Healthy() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !l.Healthy() {
		t.Fatal("not started")
	}
	if err := l.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if last.stopped.Load() != 1 {
		t.Fatal("not stopped")
	}
	if err := l.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for made.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := l.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if made.Load() != 2 {
		t.Fatalf("made=%d", made.Load())
	}
}

func TestLifecycleStopDoesNotReportNormalCancellation(t *testing.T) {
	l := NewLifecycle(func() LifecycleEngine { return &lifecycleFake{} }, RetryPolicy{HealthInterval: time.Millisecond})
	if err := l.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := l.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-l.Errors():
		t.Fatalf("normal shutdown reported lifecycle error: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestLifecycleStopAcceptsNilContext(t *testing.T) {
	l := NewLifecycle(func() LifecycleEngine { return &lifecycleFake{} }, RetryPolicy{HealthInterval: time.Millisecond})
	if err := l.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := l.Stop(nil); err != nil {
		t.Fatal(err)
	}
}
