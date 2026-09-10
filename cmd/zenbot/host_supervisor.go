package main

import (
	"context"
	"fmt"
	"sync"

	"zenbot/internal/core"
)

// hostSupervisor keeps process-owned master replacement outside commands and
// transport engines. Its callbacks are composed only by main.
type hostSupervisor struct {
	mu       sync.Mutex
	master   any
	retire   func(context.Context, any) error
	build    func(context.Context) (any, error)
	start    func(context.Context, any) error
	rebind   func(any)
	teardown sync.Once
}

func NewHostSupervisor(master any, retire func(context.Context, any) error, build func(context.Context) (any, error), start func(context.Context, any) error, rebind func(any)) *hostSupervisor {
	return &hostSupervisor{master: master, retire: retire, build: build, start: start, rebind: rebind}
}
func newProductionHostLifecycle(supervisor *hostSupervisor) *core.HostLifecycle {
	if supervisor == nil {
		return core.NewHostLifecycle(nil, nil)
	}
	return core.NewHostLifecycle(supervisor.Restart, supervisor.Shutdown)
}

func (s *hostSupervisor) Master() any { s.mu.Lock(); defer s.mu.Unlock(); return s.master }

// StartInitial uses the same fresh graph construction path as restart without
// retiring a bootstrap master.
func (s *hostSupervisor) StartInitial(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.build == nil {
		return fmt.Errorf("fresh master builder is not configured")
	}
	next, err := s.build(ctx)
	if err != nil {
		return err
	}
	if s.start != nil {
		if err := s.start(ctx, next); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.master = next
	s.mu.Unlock()
	if s.rebind != nil {
		s.rebind(next)
	}
	return nil
}
func (s *hostSupervisor) Restart(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.build == nil {
		return fmt.Errorf("fresh master builder is not configured")
	}
	s.mu.Lock()
	old := s.master
	s.mu.Unlock()
	if old != nil && s.retire != nil {
		if err := s.retire(ctx, old); err != nil {
			return err
		}
	}
	next, err := s.build(ctx)
	if err != nil {
		return err
	}
	if s.start != nil {
		if err := s.start(ctx, next); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.master = next
	s.mu.Unlock()
	if s.rebind != nil {
		s.rebind(next)
	}
	return nil
}
func (s *hostSupervisor) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	old := s.master
	s.master = nil
	s.mu.Unlock()
	if old != nil && s.retire != nil {
		return s.retire(ctx, old)
	}
	return nil
}
func (s *hostSupervisor) SignalTeardown(fn func(context.Context) error, ctx context.Context) {
	s.teardown.Do(func() {
		if fn != nil {
			_ = fn(ctx)
		}
	})
}
