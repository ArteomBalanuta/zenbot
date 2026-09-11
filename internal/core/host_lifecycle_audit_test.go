package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"zenbot/internal/common"
)

type hostLifecycleAuditEngine struct {
	common.Engine
	controller *HostLifecycle
}

func TestHostLifecycleAuditRejectsPendingTerminalClosedAndCanceledDispatch(t *testing.T) {
	for _, state := range []string{"pending", "terminal", "closed", "canceled"} {
		t.Run(state, func(t *testing.T) {
			h := NewHostLifecycle(func(context.Context) error { return nil }, func(context.Context) error { return nil })
			defer h.Close()
			release, err := h.BeginDispatch(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch state {
			case "pending":
				if err := h.RequestRestart(ctx); err != nil {
					t.Fatal(err)
				}
			case "terminal":
				if err := h.RequestShutdown(ctx); err != nil {
					t.Fatal(err)
				}
				release()
				h.Wait()
			case "closed":
				h.Close()
			case "canceled":
				cancel()
			}
			command := &hostLifecycleAuditCommand{}
			_, err = common.InvokeCommand(ctx, &hostLifecycleAuditEngine{controller: h}, command)
			if err == nil || command.calls != 0 {
				t.Fatalf("unsafe admission: calls=%d error=%v", command.calls, err)
			}
		})
	}
}

func TestHostLifecycleAuditOwnerCancellationRejectsAdmissionAndSettlesPending(t *testing.T) {
	owner, cancel := context.WithCancel(context.Background())
	h := NewHostLifecycleWithContext(owner, func(context.Context) error { t.Error("pending callback executed after owner cancellation"); return nil }, nil, nil)
	defer h.Close()
	release, err := h.BeginDispatch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	admission, err := h.RequestRestartResult(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := h.BeginDispatch(context.Background()); err == nil {
		t.Fatal("canceled owner admitted dispatch")
	}
	if err := h.RequestRestart(context.Background()); err == nil {
		t.Fatal("canceled owner admitted restart")
	}
	h.Wait()
	if result, done := admission.Operation.Result(); !done || !errors.Is(result.Err, ErrHostLifecycleClosed) {
		t.Fatalf("pending result=%+v done=%v", result, done)
	}
}

func TestHostLifecycleAuditCompletionPreservesCoalescedFailureAndOwnerContext(t *testing.T) {
	type key struct{}
	owner := context.WithValue(context.Background(), key{}, "owner")
	started, release := make(chan struct{}), make(chan struct{})
	want := errors.New("actual restart failure")
	var reported []LifecycleResult
	h := NewHostLifecycleWithContext(owner, func(ctx context.Context) error {
		if ctx.Value(key{}) != "owner" {
			t.Error("callback lost owner context")
		}
		close(started)
		<-release
		if err := ctx.Err(); err != nil {
			t.Errorf("short request canceled owned work: %v", err)
		}
		return want
	}, func(context.Context) error { return nil }, func(result LifecycleResult) { reported = append(reported, result) })
	defer h.Close()
	requestCtx, cancel := context.WithCancel(context.Background())
	first, err := h.RequestRestartResult(requestCtx)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	second, err := h.RequestRestartResult(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Coalesced || !second.Coalesced || first.Operation != second.Operation {
		t.Fatal("coalesced restart lost operation identity")
	}
	shutdown, err := h.RequestShutdownResult(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	h.Wait()
	result, done := first.Operation.Result()
	if !done || !errors.Is(result.Err, want) {
		t.Fatalf("restart result=%+v done=%v", result, done)
	}
	if result, done := shutdown.Operation.Result(); !done || result.Err != nil {
		t.Fatalf("shutdown=%+v done=%v", result, done)
	}
	if len(reported) != 2 || !errors.Is(reported[0].Err, want) {
		t.Fatalf("source reporting=%+v", reported)
	}
}

func TestHostLifecycleAuditCloseSettlesPendingAndWakesWait(t *testing.T) {
	h := NewHostLifecycle(func(context.Context) error { t.Error("closed pending request executed"); return nil }, nil)
	release, err := h.BeginDispatch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	admission, err := h.RequestRestartResult(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	waited := make(chan struct{})
	go func() { h.Wait(); close(waited) }()
	h.Close()
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("pending Close stranded Wait")
	}
	if result, done := admission.Operation.Result(); !done || !errors.Is(result.Err, ErrHostLifecycleClosed) {
		t.Fatalf("pending result=%+v done=%v", result, done)
	}
	h.Close()
}

func TestHostLifecycleAuditCloseCancelsActiveAndPanicSettles(t *testing.T) {
	for _, panicCallback := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancellation", true: "panic"}[panicCallback], func(t *testing.T) {
			started := make(chan struct{})
			h := NewHostLifecycle(func(ctx context.Context) error {
				close(started)
				if panicCallback {
					panic("callback bug")
				}
				<-ctx.Done()
				return ctx.Err()
			}, nil)
			admission, err := h.RequestRestartResult(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			<-started
			h.Close()
			h.Wait()
			result, done := admission.Operation.Result()
			if !done || result.Err == nil {
				t.Fatalf("result=%+v done=%v", result, done)
			}
			if !panicCallback && !errors.Is(result.Err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", result.Err)
			}
		})
	}
}

func TestHostLifecycleAuditReleaseExactlyOnceAndSupersededPendingSettles(t *testing.T) {
	h := NewHostLifecycle(func(context.Context) error { t.Error("superseded restart ran"); return nil }, func(context.Context) error { return nil })
	defer h.Close()
	release, err := h.BeginDispatch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := common.InvokeCommand(context.Background(), &hostLifecycleAuditEngine{controller: h}, &hostLifecycleAuditCommand{}); err != nil {
		t.Fatal(err)
	}
	restart, err := h.RequestRestartResult(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	shutdown, err := h.RequestShutdownResult(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result, done := restart.Operation.Result(); !done || !errors.Is(result.Err, ErrHostLifecycleSuperseded) {
		t.Fatalf("supersession=%+v done=%v", result, done)
	}
	release()
	release()
	h.Wait()
	if result, done := shutdown.Operation.Result(); !done || result.Err != nil {
		t.Fatalf("shutdown=%+v done=%v", result, done)
	}
}

func (e *hostLifecycleAuditEngine) HostLifecycleController() common.HostLifecycleController {
	return e.controller
}

type hostLifecycleAuditCommand struct {
	common.Command
	calls int
}

func (c *hostLifecycleAuditCommand) Execute() { c.calls++ }

func TestHostLifecycleAuditRejectsDispatchWhileRestartIsActive(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	h := NewHostLifecycle(func(context.Context) error {
		close(started)
		<-release
		return nil
	}, nil)
	defer h.Close()
	defer close(release)
	if err := h.RequestRestart(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("restart did not reach controlled callback")
	}
	command := &hostLifecycleAuditCommand{}
	_, err := common.InvokeCommand(context.Background(), &hostLifecycleAuditEngine{controller: h}, command)
	if err == nil || command.calls != 0 {
		t.Fatalf("command entered retiring host: calls=%d err=%v", command.calls, err)
	}
}

func TestHostLifecycleAuditRejectsUnavailableAndClosedRequests(t *testing.T) {
	t.Run("missing callback", func(t *testing.T) {
		h := NewHostLifecycle(nil, nil)
		defer h.Close()
		if err := h.RequestRestart(context.Background()); err == nil {
			t.Error("unconfigured restart reported acceptance")
		}
		if err := h.RequestShutdown(context.Background()); err == nil {
			t.Error("unconfigured shutdown reported acceptance")
		}
	})
	t.Run("closed owner", func(t *testing.T) {
		h := NewHostLifecycle(func(context.Context) error { return nil }, func(context.Context) error { return nil })
		h.Close()
		if err := h.RequestRestart(context.Background()); err == nil {
			t.Error("closed owner accepted a request with no worker")
		}
		if err := h.RequestShutdown(context.Background()); err == nil {
			t.Error("closed owner accepted shutdown with no worker")
		}
	})
}

func TestHostLifecycleAuditCloseIsIdempotent(t *testing.T) {
	h := NewHostLifecycle(nil, nil)
	h.Close()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Errorf("second Close panicked: %v", recovered)
		}
	}()
	h.Close()
}
