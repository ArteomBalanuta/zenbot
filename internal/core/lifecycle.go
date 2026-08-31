package core

import (
	"context"
	"errors"
	"sync"
	"time"
)

type LifecycleEngine interface {
	Start(context.Context) error
	Stop(context.Context) error
	Healthy() bool
}
type RetryPolicy struct {
	Interval, HealthInterval, StopTimeout time.Duration
	MaxRetries                            int
}
type Lifecycle struct {
	factory  func() LifecycleEngine
	policy   RetryPolicy
	mu       sync.Mutex
	engine   LifecycleEngine
	cancel   context.CancelFunc
	done     chan struct{}
	stopping bool
	errors   chan error
}

func NewLifecycle(factory func() LifecycleEngine, policy RetryPolicy) *Lifecycle {
	if policy.Interval <= 0 {
		policy.Interval = time.Second
	}
	if policy.HealthInterval <= 0 {
		policy.HealthInterval = 15 * time.Second
	}
	if policy.StopTimeout <= 0 {
		policy.StopTimeout = 10 * time.Second
	}
	return &Lifecycle{factory: factory, policy: policy, errors: make(chan error, 16)}
}
func (l *Lifecycle) Errors() <-chan error { return l.errors }
func (l *Lifecycle) report(err error) {
	if err != nil {
		select {
		case l.errors <- err:
		default:
		}
	}
}
func (l *Lifecycle) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	l.mu.Lock()
	if l.cancel != nil {
		l.mu.Unlock()
		return errors.New("lifecycle already started")
	}
	if l.factory == nil {
		l.mu.Unlock()
		return errors.New("lifecycle factory is nil")
	}
	runCtx, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	l.done = make(chan struct{})
	l.stopping = false
	done := l.done
	l.mu.Unlock()
	go func() {
		defer close(done)
		err := l.run(runCtx)
		// Context cancellation is the normal shutdown signal, not a lifecycle
		// failure. Report only terminal errors that indicate an unexpected stop.
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			l.report(err)
		}
	}()
	return nil
}
func (l *Lifecycle) run(ctx context.Context) error {
	attempts := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		e := l.factory()
		if e == nil {
			return errors.New("lifecycle factory returned nil")
		}
		l.mu.Lock()
		l.engine = e
		l.mu.Unlock()
		err := e.Start(ctx)
		if err == nil {
			attempts = 0
			err = l.monitor(ctx, e)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			// A failed health check is a bounded reconnect attempt. Stop the
			// unhealthy instance before asking the factory for a replacement.
			stopCtx, cancel := context.WithTimeout(context.Background(), l.policy.StopTimeout)
			_ = e.Stop(stopCtx)
			cancel()
			attempts++
		} else {
			attempts++
		}
		if l.policy.MaxRetries > 0 && attempts >= l.policy.MaxRetries {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.policy.Interval):
		}
	}
}
func (l *Lifecycle) monitor(ctx context.Context, e LifecycleEngine) error {
	t := time.NewTicker(l.policy.HealthInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if !e.Healthy() {
				return errors.New("lifecycle health check failed")
			}
		}
	}
}
func (l *Lifecycle) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	l.mu.Lock()
	cancel := l.cancel
	e := l.engine
	done := l.done
	l.cancel = nil
	l.engine = nil
	l.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if e != nil {
		stopCtx, c := context.WithTimeout(ctx, l.policy.StopTimeout)
		defer c()
		err := e.Stop(stopCtx)
		if done != nil {
			select {
			case <-done:
			case <-stopCtx.Done():
				if err == nil {
					err = stopCtx.Err()
				}
			}
		}
		return err
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
func closeOnce(ch chan struct{}) { defer func() { recover() }(); close(ch) }
func (l *Lifecycle) Restart(ctx context.Context) error {
	if err := l.Stop(ctx); err != nil {
		return err
	}
	return l.Start(ctx)
}
func (l *Lifecycle) Healthy() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.engine != nil && l.engine.Healthy()
}
