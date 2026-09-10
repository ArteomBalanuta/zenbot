package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"zenbot/internal/agent/observability"
)

var (
	ErrBusy   = errors.New("agent runtime admission queue is full")
	ErrClosed = errors.New("agent runtime is shut down")
)

type Config struct {
	MaxConcurrent  int
	QueueCapacity  int
	RequestTimeout time.Duration
}

type Runtime struct {
	runner         Runner
	sink           Sink
	failureSink    FailureSink
	ctx            context.Context
	cancel         context.CancelFunc
	jobs           chan Invocation
	workers        sync.WaitGroup
	executions     sync.WaitGroup
	ambientDrains  sync.WaitGroup
	slots          chan struct{}
	admission      chan struct{}
	requestTimeout time.Duration

	mu               sync.Mutex
	closed           bool
	locks            *keyedLocker
	pendingAmbient   Invocation
	ambientScheduled bool
}

func New(config Config, runner Runner, sink Sink) (*Runtime, error) {
	return NewWithFailureSink(config, runner, sink, nil)
}

func NewWithFailureSink(config Config, runner Runner, sink Sink, failureSink FailureSink) (*Runtime, error) {
	if config.MaxConcurrent < 1 {
		return nil, errors.New("max concurrent must be positive")
	}
	if config.QueueCapacity < 0 {
		return nil, errors.New("queue capacity must not be negative")
	}
	if config.RequestTimeout < 0 {
		return nil, errors.New("request timeout must not be negative")
	}
	if runner == nil {
		return nil, errors.New("runner must not be nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	jobCapacity := config.MaxConcurrent + config.QueueCapacity
	rt := &Runtime{runner: runner, sink: sink, failureSink: failureSink, ctx: ctx, cancel: cancel, jobs: make(chan Invocation, jobCapacity), slots: make(chan struct{}, config.MaxConcurrent), admission: make(chan struct{}, jobCapacity), locks: newKeyedLocker(), requestTimeout: config.RequestTimeout}
	rt.workers.Add(config.MaxConcurrent)
	for range config.MaxConcurrent {
		go rt.worker()
	}
	return rt, nil
}

// Submit admits work without blocking. Running and queued work share the bound.
func (rt *Runtime) Submit(invocation Invocation) error {
	if err := validateInvocation(invocation); err != nil {
		return err
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.closed {
		return ErrClosed
	}
	select {
	case rt.admission <- struct{}{}:
	default:
		return ErrBusy
	}
	select {
	case rt.jobs <- invocation:
		return nil
	default:
		<-rt.admission
		return ErrBusy
	}
}

func (rt *Runtime) worker() {
	defer rt.workers.Done()
	for {
		select {
		case <-rt.ctx.Done():
			return
		case invocation, ok := <-rt.jobs:
			if !ok {
				return
			}
			rt.executions.Add(1)
			go func() {
				defer rt.executions.Done()
				rt.execute(invocation, true)
			}()
		}
	}
}

func (rt *Runtime) execute(invocation Invocation, admitted bool) {
	if admitted {
		defer func() { <-rt.admission }()
	}
	release := rt.locks.Acquire(invocation.Context().MemoryKey())
	defer release()
	select {
	case rt.slots <- struct{}{}:
		defer func() { <-rt.slots }()
	case <-rt.ctx.Done():
		return
	}
	requestContext := rt.ctx
	if rt.requestTimeout > 0 {
		var cancel context.CancelFunc
		requestContext, cancel = context.WithTimeout(requestContext, rt.requestTimeout)
		defer cancel()
	}
	ctx := observability.WithRequest(requestContext, observability.Request{
		ID: invocation.RequestID(), Mode: string(invocation.Mode()),
		Room: invocation.Context().Room(), Nick: invocation.Context().Nick(),
	})
	started := time.Now()
	observability.Info(ctx, "agent.request.started",
		"queue_wait_ms", started.Sub(invocation.CreatedOn()).Milliseconds(),
		"input_chars", len([]rune(invocation.Prompt())),
		"command_originated", invocation.CommandOriginated(),
		"whisper", invocation.Context().Whisper(),
	)
	result, err := rt.runner.Run(ctx, invocation)
	if err != nil {
		observability.Error(ctx, "agent.request.failed", err, "duration_ms", time.Since(started).Milliseconds())
		shutdownCancellation := errors.Is(err, context.Canceled) && rt.ctx.Err() != nil
		if !shutdownCancellation && invocation.Mode().RequiresReply() && rt.failureSink != nil {
			rt.failureSink.DeliverFailure(ctx, invocation, err)
		}
		return
	}
	if result.ShouldReply() == false || rt.ctx.Err() != nil || rt.sink == nil {
		if result.ToolDeliveryOwned() && rt.ctx.Err() == nil {
			rt.afterDelivery(ctx, invocation, result)
		}
		observability.Info(ctx, "agent.request.completed",
			"duration_ms", time.Since(started).Milliseconds(),
			"reply", false,
			"evidence_count", len(result.DurableEvidence()),
		)
		return
	}
	observability.Debug(ctx, "agent.delivery.started", "output_chars", len([]rune(result.Text())))
	if err := rt.sink.Deliver(ctx, invocation, result); err != nil {
		observability.Error(ctx, "agent.delivery.failed", err, "duration_ms", time.Since(started).Milliseconds())
		return
	}
	observability.Info(ctx, "agent.delivery.completed", "output_chars", len([]rune(result.Text())))
	if !rt.afterDelivery(ctx, invocation, result) {
		return
	}
	observability.Info(ctx, "agent.request.completed",
		"duration_ms", time.Since(started).Milliseconds(),
		"reply", true,
		"evidence_count", len(result.DurableEvidence()),
	)
}

func (rt *Runtime) afterDelivery(ctx context.Context, invocation Invocation, result Result) bool {
	after, ok := rt.runner.(interface {
		AfterDelivery(context.Context, Invocation, Result) error
	})
	if !ok {
		return true
	}
	if err := after.AfterDelivery(ctx, invocation, result); err != nil {
		if strings.Contains(err.Error(), "agent tool evidence persistence failed") {
			observability.Error(ctx, "agent.evidence.persistence_failed", err)
		} else {
			observability.Error(ctx, "agent.memory.persistence_failed", err)
		}
		return false
	}
	observability.Debug(ctx, "agent.persistence.completed", "evidence_count", len(result.DurableEvidence()))
	return true
}

// SubmitAmbient retains only the latest pending ambient request. Unlike Submit,
// it does not consume reply-required admission capacity.
func (rt *Runtime) SubmitAmbient(invocation Invocation) error {
	if err := validateInvocation(invocation); err != nil {
		return err
	}
	if invocation.Mode() != AMBIENT {
		return errors.New("ambient submission requires AMBIENT mode")
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.closed {
		return ErrClosed
	}
	rt.pendingAmbient = invocation
	if rt.ambientScheduled {
		return nil
	}
	rt.ambientScheduled = true
	rt.ambientDrains.Add(1)
	go rt.drainAmbient()
	return nil
}

func (rt *Runtime) drainAmbient() {
	defer rt.ambientDrains.Done()
	executed := false
	for {
		if executed {
			rt.waitForOrdinary()
		}
		rt.mu.Lock()
		if rt.closed {
			rt.pendingAmbient = Invocation{}
			rt.ambientScheduled = false
			rt.mu.Unlock()
			return
		}
		invocation := rt.pendingAmbient
		rt.pendingAmbient = Invocation{}
		if invocation.RequestID() == "" {
			rt.ambientScheduled = false
			rt.mu.Unlock()
			return
		}
		rt.mu.Unlock()
		rt.execute(invocation, false)
		executed = true
	}
}

func (rt *Runtime) waitForOrdinary() {
	for len(rt.admission) != 0 {
		select {
		case <-rt.ctx.Done():
			return
		case <-time.After(time.Millisecond):
		}
	}
}

// Close cancels in-flight runners, prevents new admission, and waits for workers.
// Runners are required to honor cancellation for shutdown to complete promptly.
func (rt *Runtime) Close() {
	rt.mu.Lock()
	if rt.closed {
		rt.mu.Unlock()
		return
	}
	rt.closed = true
	rt.pendingAmbient = Invocation{}
	close(rt.jobs)
	rt.cancel()
	rt.mu.Unlock()
	rt.workers.Wait()
	rt.executions.Wait()
	rt.ambientDrains.Wait()
}
