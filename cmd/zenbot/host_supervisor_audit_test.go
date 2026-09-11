package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"zenbot/internal/core"
)

func TestHostSupervisorAuditFailedReplacementUnpublishesRetiredHost(t *testing.T) {
	old := &supervisorMasterFake{id: 1}
	want := errors.New("replacement construction failed")
	retired := false
	bound := any(old)
	s := NewHostSupervisor(old,
		func(context.Context, any) error { retired = true; return nil },
		func(context.Context) (any, error) { return nil, want },
		nil,
		func(next any) { bound = next },
	)
	err := s.Restart(context.Background())
	if !errors.Is(err, want) || !retired {
		t.Fatalf("fixture did not reach failed replacement after retirement: err=%v retired=%v", err, retired)
	}
	if s.Master() != nil || bound != nil {
		t.Fatalf("retired host remains published after failed replacement: master=%v binding=%v", s.Master(), bound)
	}
}

func TestHostSupervisorAuditFailureCleansCandidateAndWithdrawsBindings(t *testing.T) {
	for _, stage := range []string{"build", "start", "retire", "canceled"} {
		t.Run(stage, func(t *testing.T) {
			old, candidate := &supervisorMasterFake{1}, &supervisorMasterFake{2}
			want := errors.New(stage + " failure")
			bound := any(old)
			var retired []any
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := NewHostSupervisor(old, func(_ context.Context, host any) error {
				retired = append(retired, host)
				if stage == "retire" {
					return want
				}
				return nil
			}, func(context.Context) (any, error) {
				if stage == "build" {
					return candidate, want
				}
				if stage == "canceled" {
					cancel()
				}
				return candidate, nil
			}, func(context.Context, any) error {
				if stage == "start" {
					return want
				}
				return nil
			}, func(host any) { bound = host })
			err := s.Restart(ctx)
			if stage == "canceled" {
				want = context.Canceled
			}
			if !errors.Is(err, want) {
				t.Fatalf("source error lost: %v", err)
			}
			if s.Master() != nil || bound != nil {
				t.Fatalf("failed host published: %v %v", s.Master(), bound)
			}
			wantRetired := []any{old, candidate}
			if stage == "retire" {
				wantRetired = []any{old}
			}
			if !reflect.DeepEqual(retired, wantRetired) {
				t.Fatalf("cleanup=%v want=%v", retired, wantRetired)
			}
		})
	}
}

func TestHostSupervisorAuditCancellationDuringBuildCannotPublishCandidate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	building, finish := make(chan struct{}), make(chan struct{})
	candidate := &supervisorMasterFake{2}
	var started, retired int
	bound := any(nil)
	s := NewHostSupervisor(nil, func(_ context.Context, host any) error { retired++; return nil }, func(context.Context) (any, error) {
		close(building)
		<-finish
		return candidate, nil
	}, func(context.Context, any) error { started++; return nil }, func(host any) { bound = host })
	restartDone := make(chan error, 1)
	go func() { restartDone <- s.Restart(ctx) }()
	<-building
	cancel()
	close(finish)
	if err := <-restartDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("restart error=%v", err)
	}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Master() != nil || bound != nil || started != 0 || retired != 1 {
		t.Fatalf("master=%v bound=%v started=%d retired=%d", s.Master(), bound, started, retired)
	}
}

func TestHostSupervisorAuditSignalTeardownCancelsAndSerializesBuildingCandidate(t *testing.T) {
	building, canceled, finish := make(chan struct{}), make(chan struct{}), make(chan struct{})
	candidate := &supervisorMasterFake{2}
	var started, retired, published int
	s := NewHostSupervisor(nil, func(ctx context.Context, host any) error {
		if ctx.Err() != nil {
			t.Error("cleanup inherited canceled build context")
		}
		retired++
		return nil
	}, func(ctx context.Context) (any, error) {
		close(building)
		<-ctx.Done()
		close(canceled)
		<-finish
		return candidate, nil
	}, func(context.Context, any) error { started++; return nil }, func(host any) {
		if host != nil {
			published++
		}
	})
	initialDone := make(chan error, 1)
	go func() { initialDone <- s.StartInitial(context.Background()) }()
	<-building
	teardownDone := make(chan error, 1)
	go func() { teardownDone <- s.SignalTeardown(s.Shutdown, context.Background()) }()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		close(finish)
		t.Fatal("teardown did not cancel active construction")
	}
	select {
	case err := <-teardownDone:
		t.Fatalf("teardown returned before construction quiesced: %v", err)
	default:
	}
	close(finish)
	if err := <-initialDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("initial error=%v", err)
	}
	if err := <-teardownDone; err != nil {
		t.Fatal(err)
	}
	if started != 0 || retired != 1 || published != 0 || s.Master() != nil {
		t.Fatalf("start=%d retire=%d published=%d master=%v", started, retired, published, s.Master())
	}
	if err := s.Restart(context.Background()); err == nil {
		t.Fatal("teardown admitted replacement")
	}
}

func TestHostSupervisorAuditFailedCleanupRetainsOwnerForFinalTeardown(t *testing.T) {
	candidate := &supervisorMasterFake{2}
	startFailure, cleanupFailure := errors.New("start failed"), errors.New("cleanup failed")
	var builds, retires int
	s := NewHostSupervisor(nil, func(context.Context, any) error {
		retires++
		if retires == 1 {
			return cleanupFailure
		}
		return nil
	}, func(context.Context) (any, error) { builds++; return candidate, nil }, func(context.Context, any) error { return startFailure }, nil)
	err := s.StartInitial(context.Background())
	if !errors.Is(err, startFailure) || !errors.Is(err, cleanupFailure) {
		t.Fatalf("failure evidence lost: %v", err)
	}
	if s.Master() != nil {
		t.Fatal("failed candidate published")
	}
	if retry := s.Restart(context.Background()); retry == nil || builds != 1 || retires != 1 {
		t.Fatalf("failed source replayed: retry=%v builds=%d retires=%d", retry, builds, retires)
	}
	if err := s.SignalTeardown(s.Shutdown, context.Background()); err != nil {
		t.Fatal(err)
	}
	if retires != 2 {
		t.Fatalf("owned candidate not cleaned: retires=%d", retires)
	}
	if !errors.Is(err, startFailure) || !errors.Is(err, cleanupFailure) {
		t.Fatal("later cleanup erased original operation error")
	}
}

func TestHostSupervisorAuditSuccessfulHostOutlivesStartupCall(t *testing.T) {
	var lifetime context.Context
	s := NewHostSupervisor(nil, func(context.Context, any) error { return nil }, func(ctx context.Context) (any, error) {
		lifetime = ctx
		return &supervisorMasterFake{1}, nil
	}, nil, nil)
	if err := s.StartInitial(context.Background()); err != nil {
		t.Fatal(err)
	}
	if lifetime.Err() != nil {
		t.Fatal("successful host canceled on startup return")
	}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if lifetime.Err() != context.Canceled {
		t.Fatal("shutdown did not cancel owned host")
	}
}

func TestHostSupervisorAuditStartPanicCleansCandidate(t *testing.T) {
	var retired int
	s := NewHostSupervisor(nil, func(context.Context, any) error { retired++; return nil }, func(context.Context) (any, error) { return &supervisorMasterFake{1}, nil }, func(context.Context, any) error { panic("start panic") }, nil)
	h := newProductionHostLifecycle(context.Background(), s, nil)
	defer h.Close()
	admission, err := h.RequestRestartResult(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	h.Wait()
	result, done := admission.Operation.Result()
	if !done || result.Err == nil || retired != 1 || s.Master() != nil {
		t.Fatalf("panic cleanup result=%+v done=%v retired=%d master=%v", result, done, retired, s.Master())
	}
}

func TestHostSupervisorAuditProcessCancellationQuiescesLifecycleBeforeTeardown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	building, canceled, finish := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var retired, started, reported int
	s := NewHostSupervisor(nil, func(context.Context, any) error { retired++; return nil }, func(ctx context.Context) (any, error) {
		close(building)
		<-ctx.Done()
		close(canceled)
		<-finish
		return &supervisorMasterFake{2}, nil
	}, func(context.Context, any) error { started++; return nil }, nil)
	h := newProductionHostLifecycle(ctx, s, func(result core.LifecycleResult) {
		if errors.Is(result.Err, context.Canceled) {
			reported++
		}
	})
	defer h.Close()
	admission, err := h.RequestRestartResult(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	<-building
	cancel()
	<-canceled
	teardownDone := make(chan error, 1)
	go func() { h.Close(); teardownDone <- s.SignalTeardown(s.Shutdown, context.Background()) }()
	select {
	case err := <-teardownDone:
		t.Fatalf("teardown outran lifecycle owner: %v", err)
	default:
	}
	close(finish)
	if err := <-teardownDone; err != nil {
		t.Fatal(err)
	}
	if result, done := admission.Operation.Result(); !done || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("source cancellation=%+v done=%v", result, done)
	}
	if s.Master() != nil || started != 0 || retired != 1 || reported != 1 {
		t.Fatalf("master=%v start=%d retire=%d reported=%d", s.Master(), started, retired, reported)
	}
}
