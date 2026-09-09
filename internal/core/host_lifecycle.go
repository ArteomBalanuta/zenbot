package core

import (
	"context"
	"sync"
)

// HostLifecycle serializes process-owned lifecycle callbacks. It owns no
// process resources; callers inject restart and shutdown work at composition.
type HostLifecycle struct {
	mu         sync.Mutex
	restart    func(context.Context) error
	shutdown   func(context.Context) error
	pending    lifecycleRequest
	active     lifecycleRequest
	dispatches int
	terminal   bool
	wake       chan struct{}
	done       chan struct{}
	idle       *sync.Cond
	wg         sync.WaitGroup
}

type lifecycleRequest uint8

const (
	noLifecycleRequest lifecycleRequest = iota
	restartRequest
	shutdownRequest
)

func NewHostLifecycle(restart, shutdown func(context.Context) error) *HostLifecycle {
	h := &HostLifecycle{restart: restart, shutdown: shutdown, wake: make(chan struct{}, 1), done: make(chan struct{})}
	h.idle = sync.NewCond(&h.mu)
	h.wg.Add(1)
	go h.run()
	return h
}

func (h *HostLifecycle) BeginDispatch() func() {
	h.mu.Lock()
	h.dispatches++
	h.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			h.dispatches--
			h.signal()
			h.mu.Unlock()
		})
	}
}

func (h *HostLifecycle) RequestRestart(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	h.mu.Lock()
	if !h.terminal && h.pending != shutdownRequest && h.active != restartRequest {
		h.pending = restartRequest
		h.signal()
	}
	h.mu.Unlock()
	return nil
}

func (h *HostLifecycle) RequestShutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	h.mu.Lock()
	if !h.terminal {
		h.terminal = true
		h.pending = shutdownRequest
		h.signal()
	}
	h.mu.Unlock()
	return nil
}

func (h *HostLifecycle) signal() {
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

func (h *HostLifecycle) run() {
	defer h.wg.Done()
	for {
		select {
		case <-h.wake:
		case <-h.done:
			return
		}
		h.mu.Lock()
		if h.dispatches != 0 {
			h.mu.Unlock()
			continue
		}
		req := h.pending
		h.pending = noLifecycleRequest
		h.active = req
		h.mu.Unlock()
		switch req {
		case restartRequest:
			if h.restart != nil {
				_ = h.restart(context.Background())
			}
		case shutdownRequest:
			if h.shutdown != nil {
				_ = h.shutdown(context.Background())
			}
		}
		h.mu.Lock()
		h.active = noLifecycleRequest
		h.idle.Broadcast()
		h.mu.Unlock()
	}
}

// Wait waits until no admitted lifecycle request remains active or queued.
func (h *HostLifecycle) Wait() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for h.pending != noLifecycleRequest || h.active != noLifecycleRequest {
		h.idle.Wait()
	}
}

func (h *HostLifecycle) Close() { close(h.done); h.wg.Wait() }
