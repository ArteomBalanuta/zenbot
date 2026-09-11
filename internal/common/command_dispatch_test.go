package common

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/model"
)

type invocationLease struct {
	active, acquired, released int
	onBegin                    func()
}

func (l *invocationLease) BeginDispatch(context.Context) (func(), error) {
	l.active++
	l.acquired++
	if l.onBegin != nil {
		l.onBegin()
	}
	return func() { l.active--; l.released++ }, nil
}
func (*invocationLease) RequestRestart(context.Context) error  { return nil }
func (*invocationLease) RequestShutdown(context.Context) error { return nil }

type invocationEngine struct {
	Engine
	lease *invocationLease
}

func (e *invocationEngine) HostLifecycleController() HostLifecycleController { return e.lease }

type voidInvocation struct {
	Command
	calls int
	run   func()
}

func (c *voidInvocation) Execute() {
	c.calls++
	if c.run != nil {
		c.run()
	}
}

type contextualInvocation struct {
	voidInvocation
	ctx context.Context
}

func (c *contextualInvocation) ExecuteContext(ctx context.Context) { c.ctx = ctx; c.calls++ }

type resultInvocation struct {
	contextualInvocation
	status      model.Status
	failure     error
	resultCalls int
	onResult    func(context.Context)
}

func (c *resultInvocation) ExecuteResult(ctx context.Context) (model.Status, error) {
	c.resultCalls++
	if c.onResult != nil {
		c.onResult(ctx)
	}
	return c.status, c.failure
}

func TestInvokeCommandKeepsLegacyOutcomeUnknown(t *testing.T) {
	legacy := &voidInvocation{}
	status, err := InvokeCommand(nil, nil, legacy)
	if status != "" || err != nil || legacy.calls != 1 {
		t.Fatalf("legacy status=%s err=%v calls=%d", status, err, legacy.calls)
	}
	contextual := &contextualInvocation{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	status, err = InvokeCommand(ctx, nil, contextual)
	if status != "" || err != nil || contextual.calls != 1 || contextual.ctx != ctx {
		t.Fatalf("contextual status=%s err=%v command=%+v", status, err, contextual)
	}
}

func TestInvokeCommandResultPrecedenceAndLeaseUnwinding(t *testing.T) {
	failure := errors.New("execution failed")
	for _, outcome := range []string{"success", "error", "rejection", "panic"} {
		t.Run(outcome, func(t *testing.T) {
			lease := &invocationLease{}
			engine := &invocationEngine{lease: lease}
			command := &resultInvocation{status: model.SUCCESSFUL}
			if outcome == "error" {
				command.status = model.FAILED
				command.failure = failure
			}
			if outcome == "rejection" {
				command.status = model.FAILED
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			command.onResult = func(caller context.Context) {
				if lease.active != 1 || caller != ctx {
					t.Errorf("execution outside lease/caller: active=%d ctx=%v", lease.active, caller)
				}
				if outcome == "panic" {
					panic("execution panic")
				}
			}
			func() {
				if outcome == "panic" {
					defer func() {
						if recover() != "execution panic" {
							t.Error("panic not propagated")
						}
					}()
				}
				status, err := InvokeCommand(ctx, engine, command)
				if status != command.status {
					t.Errorf("status=%s want=%s", status, command.status)
				}
				switch outcome {
				case "success":
					if err != nil {
						t.Error(err)
					}
				case "error":
					if !errors.Is(err, failure) {
						t.Errorf("err=%v", err)
					}
				case "rejection":
					var rejection *CommandRejectedError
					if !errors.As(err, &rejection) || rejection.Status != model.FAILED {
						t.Errorf("rejection=%v", err)
					}
				}
			}()
			if command.resultCalls != 1 || command.calls != 0 || lease.active != 0 || lease.acquired != 1 || lease.released != 1 {
				t.Fatalf("command=%+v lease=%+v", command, lease)
			}
		})
	}
}

func TestInvokeCommandCancellationAtLeaseBoundaryStopsExecution(t *testing.T) {
	for _, beforeBegin := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		lease := &invocationLease{onBegin: cancel}
		if beforeBegin {
			cancel()
		}
		command := &voidInvocation{}
		status, err := InvokeCommand(ctx, &invocationEngine{lease: lease}, command)
		wantLeases := 1
		if beforeBegin {
			wantLeases = 0
		}
		if status != model.FAILED || err != context.Canceled || command.calls != 0 || lease.active != 0 || lease.acquired != wantLeases || lease.released != wantLeases {
			t.Fatalf("status=%s err=%v calls=%d lease=%+v", status, err, command.calls, lease)
		}
	}
}
