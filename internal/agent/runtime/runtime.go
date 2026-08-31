package runtime

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"
)

var (
	ErrBusy   = errors.New("agent runtime admission queue is full")
	ErrClosed = errors.New("agent runtime is shut down")
)

type Config struct {
	MaxConcurrent int
	QueueCapacity int
}

type Runtime struct {
	runner        Runner
	sink          Sink
	failureSink   FailureSink
	ctx           context.Context
	cancel        context.CancelFunc
	jobs          chan Invocation
	workers       sync.WaitGroup
	executions    sync.WaitGroup
	ambientDrains sync.WaitGroup
	slots         chan struct{}
	admission     chan struct{}

	mu               sync.Mutex
	closed           bool
	rooms            map[string]*sync.Mutex
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
	if runner == nil {
		return nil, errors.New("runner must not be nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	jobCapacity := config.MaxConcurrent + config.QueueCapacity
	rt := &Runtime{runner: runner, sink: sink, failureSink: failureSink, ctx: ctx, cancel: cancel, jobs: make(chan Invocation, jobCapacity), slots: make(chan struct{}, config.MaxConcurrent), admission: make(chan struct{}, jobCapacity), rooms: make(map[string]*sync.Mutex)}
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
	room := rt.roomLock(invocation.Context().MemoryKey())
	room.Lock()
	defer room.Unlock()
	select {
	case rt.slots <- struct{}{}:
		defer func() { <-rt.slots }()
	case <-rt.ctx.Done():
		return
	}
	result, err := rt.runner.Run(rt.ctx, invocation)
	if err != nil {
		if rt.ctx.Err() == nil && invocation.Mode().RequiresReply() && rt.failureSink != nil {
			rt.failureSink.DeliverFailure(rt.ctx, invocation, err)
		}
		return
	}
	if result.ShouldReply() == false || rt.ctx.Err() != nil || rt.sink == nil {
		return
	}
	if err := rt.sink.Deliver(rt.ctx, invocation, result); err != nil {
		return
	}
	if after, ok := rt.runner.(interface {
		AfterDelivery(context.Context, Invocation, Result) error
	}); ok {
		if err := after.AfterDelivery(rt.ctx, invocation, result); err != nil {
			if strings.Contains(err.Error(), "agent tool evidence persistence failed") {
				log.Printf("agent tool evidence persistence failed")
			} else {
				log.Printf("agent memory persistence failed correlation=%s", result.CorrelationID())
			}
		}
	}
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

func (rt *Runtime) roomLock(room string) *sync.Mutex {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	lock := rt.rooms[room]
	if lock == nil {
		lock = &sync.Mutex{}
		rt.rooms[room] = lock
	}
	return lock
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
