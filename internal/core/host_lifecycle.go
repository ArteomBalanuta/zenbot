package core

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"zenbot/internal/common"
)

var (
	ErrHostLifecycleClosed      = errors.New("host lifecycle is closed")
	ErrHostLifecycleBusy        = errors.New("host lifecycle is pending or active")
	ErrHostLifecycleTerminal    = errors.New("host lifecycle is terminal")
	ErrHostLifecycleUnavailable = errors.New("host lifecycle callback is unavailable")
	ErrHostLifecycleSuperseded  = errors.New("host lifecycle request was superseded by shutdown")
)

// LifecycleResult is source completion evidence, separate from admission.
type LifecycleResult = common.LifecycleResult

// LifecycleOperation is shared by coalesced requests. Callers may retain their
// handle; the owner retains only pending and active operations. A dispatcher
// must return and release its lease before waiting for this completion.
type LifecycleOperation struct {
	mu       sync.RWMutex
	done     chan struct{}
	result   LifecycleResult
	finished bool
}

func (o *LifecycleOperation) Done() <-chan struct{} { return o.done }
func (o *LifecycleOperation) Result() (LifecycleResult, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.result, o.finished
}
func (o *LifecycleOperation) finish(err error) LifecycleResult {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.finished {
		o.result.Err = err
		o.finished = true
		close(o.done)
	}
	return o.result
}

type LifecycleAdmission = common.LifecycleAdmission

// HostLifecycle owns command admission and serial process-lifetime callbacks.
// It never waits for lifecycle work from a submitting command's dispatcher.
type HostLifecycle struct {
	mu                sync.Mutex
	ctx               context.Context
	cancel            context.CancelFunc
	restart, shutdown func(context.Context) error
	report            func(LifecycleResult)
	pending, active   *LifecycleOperation
	dispatches        int
	terminal, closed  bool
	wake              chan struct{}
	idle              *sync.Cond
	wg                sync.WaitGroup
}

func NewHostLifecycle(restart, shutdown func(context.Context) error) *HostLifecycle {
	return NewHostLifecycleWithContext(context.Background(), restart, shutdown, nil)
}

// The reporter runs synchronously outside owner locks and must be safe for
// concurrent calls. It must not wait for lifecycle work or close this owner.
func NewHostLifecycleWithContext(ctx context.Context, restart, shutdown func(context.Context) error, report func(LifecycleResult)) *HostLifecycle {
	if ctx == nil {
		ctx = context.Background()
	}
	owned, cancel := context.WithCancel(ctx)
	h := &HostLifecycle{ctx: owned, cancel: cancel, restart: restart, shutdown: shutdown, report: report, wake: make(chan struct{}, 1)}
	h.idle = sync.NewCond(&h.mu)
	h.wg.Add(1)
	go h.run()
	return h
}

func (h *HostLifecycle) BeginDispatch(ctx context.Context) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if h == nil {
		return nil, ErrHostLifecycleUnavailable
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.admissionError(); err != nil {
		return nil, err
	}
	if h.pending != nil || h.active != nil {
		return nil, ErrHostLifecycleBusy
	}
	h.dispatches++
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.dispatches--
			h.signal()
		})
	}, nil
}

func (h *HostLifecycle) admissionError() error {
	if h.closed || h.ctx.Err() != nil {
		return ErrHostLifecycleClosed
	}
	if h.terminal {
		return ErrHostLifecycleTerminal
	}
	return nil
}

// RequestRestart and RequestShutdown report acceptance only, never completion.
func (h *HostLifecycle) RequestRestart(ctx context.Context) error {
	_, err := h.RequestRestartResult(ctx)
	return err
}
func (h *HostLifecycle) RequestShutdown(ctx context.Context) error {
	_, err := h.RequestShutdownResult(ctx)
	return err
}
func (h *HostLifecycle) RequestRestartResult(ctx context.Context) (LifecycleAdmission, error) {
	return h.request(ctx, "restart")
}
func (h *HostLifecycle) RequestShutdownResult(ctx context.Context) (LifecycleAdmission, error) {
	return h.request(ctx, "shutdown")
}

func (h *HostLifecycle) request(ctx context.Context, kind string) (LifecycleAdmission, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return LifecycleAdmission{}, err
	}
	if h == nil {
		return LifecycleAdmission{}, ErrHostLifecycleUnavailable
	}
	h.mu.Lock()
	if h.closed || h.ctx.Err() != nil {
		h.mu.Unlock()
		return LifecycleAdmission{}, ErrHostLifecycleClosed
	}
	if (kind == "restart" && h.restart == nil) || (kind == "shutdown" && h.shutdown == nil) {
		h.mu.Unlock()
		return LifecycleAdmission{}, ErrHostLifecycleUnavailable
	}
	if h.terminal && kind != "shutdown" {
		h.mu.Unlock()
		return LifecycleAdmission{}, ErrHostLifecycleTerminal
	}
	for _, operation := range []*LifecycleOperation{h.active, h.pending} {
		if operation != nil && operation.result.Kind == kind {
			h.mu.Unlock()
			return LifecycleAdmission{Operation: operation, Coalesced: true}, nil
		}
	}
	if h.terminal {
		h.mu.Unlock()
		return LifecycleAdmission{}, ErrHostLifecycleTerminal
	}
	if kind == "shutdown" {
		h.terminal = true
	}
	superseded := h.pending
	operation := &LifecycleOperation{done: make(chan struct{}), result: LifecycleResult{Kind: kind}}
	h.pending = operation
	common.RecordCommittedMutation(ctx)
	var previous LifecycleResult
	if superseded != nil {
		previous = superseded.finish(ErrHostLifecycleSuperseded)
	}
	h.signal()
	h.mu.Unlock()
	if superseded != nil && h.report != nil {
		h.report(previous)
	}
	return LifecycleAdmission{Operation: operation}, nil
}

func (h *HostLifecycle) signal() {
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

func (h *HostLifecycle) run() {
	defer h.wg.Done()
	defer h.cancel()
	for {
		select {
		case <-h.wake:
		case <-h.ctx.Done():
		}
		h.mu.Lock()
		if h.closed || h.ctx.Err() != nil {
			h.closed = true
			pending := h.pending
			h.mu.Unlock()
			if pending != nil {
				h.complete(pending, ErrHostLifecycleClosed)
			}
			h.mu.Lock()
			h.pending = nil
			h.idle.Broadcast()
			h.mu.Unlock()
			return
		}
		if h.dispatches != 0 || h.pending == nil {
			h.mu.Unlock()
			continue
		}
		operation := h.pending
		h.pending, h.active = nil, operation
		h.mu.Unlock()
		callback := h.restart
		if operation.result.Kind == "shutdown" {
			callback = h.shutdown
		}
		h.complete(operation, callHostLifecycle(h.ctx, callback))
		h.mu.Lock()
		h.active = nil
		h.idle.Broadcast()
		if h.pending != nil {
			h.signal()
		}
		h.mu.Unlock()
	}
}

func callHostLifecycle(ctx context.Context, callback func(context.Context) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("host lifecycle callback panicked: %v", recovered)
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	return callback(ctx)
}

func (h *HostLifecycle) complete(operation *LifecycleOperation, err error) {
	result := operation.finish(err)
	if h.report != nil {
		h.report(result)
	}
}

// Wait is for process owners and observers, never an admitted dispatcher.
func (h *HostLifecycle) Wait() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for h.pending != nil || h.active != nil {
		h.idle.Wait()
	}
}

// Terminal includes process cancellation and closure, suppressing recovery.
func (h *HostLifecycle) Terminal() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.terminal || h.closed || h.ctx.Err() != nil
}

// Close cancels owned work and waits for callbacks to quiesce. It does not wait
// for dispatch leases; canceled pending requests need no retiring callback.
func (h *HostLifecycle) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.closed = true
	h.cancel()
	h.signal()
	h.mu.Unlock()
	h.wg.Wait()
}
