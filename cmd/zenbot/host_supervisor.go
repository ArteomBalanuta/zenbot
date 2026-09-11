package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"zenbot/internal/core"
)

// hostSupervisor owns construction, retirement and publication. opMu serializes
// resource work; mu lets teardown close admission and cancel a blocked builder.
type hostSupervisor struct {
	opMu                          sync.Mutex
	mu                            sync.Mutex
	master                        any
	unretired                     any
	masterCancel, candidateCancel context.CancelFunc
	closed                        bool
	retire                        func(context.Context, any) error
	build                         func(context.Context) (any, error)
	start                         func(context.Context, any) error
	rebind                        func(any)
	teardown                      sync.Once
	teardownErr                   error
}

func NewHostSupervisor(master any, retire func(context.Context, any) error, build func(context.Context) (any, error), start func(context.Context, any) error, rebind func(any)) *hostSupervisor {
	return &hostSupervisor{master: master, retire: retire, build: build, start: start, rebind: rebind}
}
func newProductionHostLifecycle(ctx context.Context, supervisor *hostSupervisor, report func(core.LifecycleResult)) *core.HostLifecycle {
	if supervisor == nil {
		return core.NewHostLifecycleWithContext(ctx, nil, nil, report)
	}
	return core.NewHostLifecycleWithContext(ctx, supervisor.Restart, supervisor.Shutdown, report)
}

func (s *hostSupervisor) Master() any { s.mu.Lock(); defer s.mu.Unlock(); return s.master }

func (s *hostSupervisor) StartInitial(ctx context.Context) error { return s.replace(ctx, true) }
func (s *hostSupervisor) Restart(ctx context.Context) error      { return s.replace(ctx, false) }

func (s *hostSupervisor) replace(ctx context.Context, initial bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return core.ErrHostLifecycleClosed
	}
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}
	if s.build == nil {
		s.mu.Unlock()
		return fmt.Errorf("fresh master builder is not configured")
	}
	if s.unretired != nil {
		s.mu.Unlock()
		return fmt.Errorf("previous host cleanup is incomplete")
	}
	if initial && s.master != nil {
		s.mu.Unlock()
		return fmt.Errorf("initial master is already constructed")
	}
	owned, cancel := context.WithCancel(ctx)
	s.candidateCancel = cancel
	old := s.master
	oldCancel := s.masterCancel
	s.master, s.masterCancel = nil, nil
	if old != nil && s.rebind != nil {
		s.rebind(nil)
	}
	s.mu.Unlock()
	published := false
	defer func() {
		s.mu.Lock()
		s.candidateCancel = nil
		s.mu.Unlock()
		if !published {
			cancel()
		}
	}()
	if oldCancel != nil {
		oldCancel()
	}
	if old != nil {
		if err := s.retireHost(owned, old); err != nil {
			return err
		}
	}
	if err := owned.Err(); err != nil {
		return err
	}
	var next any
	err := callHostSupervisor(func() error {
		var buildErr error
		next, buildErr = s.build(owned)
		return buildErr
	})
	if err == nil && next == nil {
		err = fmt.Errorf("fresh master builder returned no host")
	}
	if err == nil {
		err = owned.Err()
	}
	if err == nil && s.start != nil {
		err = callHostSupervisor(func() error { return s.start(owned, next) })
	}
	if err == nil {
		err = owned.Err()
	}
	if err != nil {
		cancel()
		return errors.Join(err, s.cleanupCandidate(ctx, next))
	}
	s.mu.Lock()
	if s.closed || owned.Err() != nil {
		s.mu.Unlock()
		cancel()
		return errors.Join(core.ErrHostLifecycleClosed, owned.Err(), s.cleanupCandidate(ctx, next))
	}
	s.master, s.masterCancel = next, cancel
	if s.rebind != nil {
		s.rebind(next)
	}
	published = true
	s.mu.Unlock()
	return nil
}

// retireHost retains failed cleanup ownership without advertising a usable host.
// Only final shutdown can attempt cleanup again; replacement does not retry it.
// Injected retire callbacks must support idempotent final cleanup (as the
// production EngineImpl.StopContext does).
func (s *hostSupervisor) retireHost(ctx context.Context, host any) error {
	var err error
	if s.retire != nil {
		err = callHostSupervisor(func() error { return s.retire(ctx, host) })
	}
	s.mu.Lock()
	if err != nil {
		s.unretired = host
	} else {
		s.unretired = nil
	}
	s.mu.Unlock()
	return err
}

func callHostSupervisor(callback func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("host supervisor callback panicked: %v", recovered)
		}
	}()
	return callback()
}

func (s *hostSupervisor) cleanupCandidate(ctx context.Context, candidate any) error {
	if candidate == nil {
		return nil
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return s.retireHost(cleanup, candidate)
}

func (s *hostSupervisor) closeAdmission() {
	s.mu.Lock()
	s.closed = true
	if s.candidateCancel != nil {
		s.candidateCancel()
	}
	if s.masterCancel != nil {
		s.masterCancel()
	}
	s.mu.Unlock()
}

// Shutdown stops only the host. Process replicas and the room agent remain
// owned by SignalTeardown's composition callback.
func (s *hostSupervisor) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.closeAdmission()
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	old := s.master
	if old == nil {
		old = s.unretired
	}
	s.master, s.masterCancel = nil, nil
	if s.rebind != nil {
		s.rebind(nil)
	}
	s.mu.Unlock()
	if old != nil {
		return s.retireHost(ctx, old)
	}
	return nil
}

func (s *hostSupervisor) SignalTeardown(fn func(context.Context) error, ctx context.Context) error {
	s.teardown.Do(func() {
		s.closeAdmission()
		if fn != nil {
			s.teardownErr = fn(ctx)
		}
	})
	return s.teardownErr
}
