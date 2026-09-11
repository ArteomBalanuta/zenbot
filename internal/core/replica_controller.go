package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type ReplicaConstructor func(context.Context, string) (ManagedEngine, error)

type ManagedReplicaController struct {
	manager   *ReplicaManager
	construct ReplicaConstructor
	errors    chan<- error
}

func NewManagedReplicaController(manager *ReplicaManager, construct ReplicaConstructor, sinks ...chan<- error) *ManagedReplicaController {
	var sink chan<- error
	if len(sinks) > 0 {
		sink = sinks[0]
	}
	return &ManagedReplicaController{manager: manager, construct: construct, errors: sink}
}
func (c *ManagedReplicaController) report(err error) {
	if c.errors != nil && err != nil {
		select {
		case c.errors <- err:
		default:
		}
	}
}
func (c *ManagedReplicaController) AddReplica(ctx context.Context, channel string) error {
	if c == nil || c.manager == nil || c.construct == nil {
		return fmt.Errorf("replica controller is not configured")
	}
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return fmt.Errorf("replica channel is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	e, err := c.construct(ctx, channel)
	if err != nil {
		c.report(fmt.Errorf("replica %s construction: %w", channel, err))
		return err
	}
	// Keep request cancellation during startup, then transfer the successful
	// lifetime to the manager. A master replacement cannot cancel this scope.
	runtimeCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	detach := context.AfterFunc(ctx, cancel)
	defer detach()
	r := &ownedReplica{managedReplica: managedReplica{e}, cancel: cancel}
	cleanup := func() {
		stopCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = r.Stop(stopCtx)
	}
	var registrationMu sync.Mutex
	var failed error
	if impl, ok := e.(*EngineImpl); ok {
		impl.SetRuntimeFailureHandler(func(runtimeErr error) {
			registrationMu.Lock()
			failed = runtimeErr
			registrationMu.Unlock()
			c.report(fmt.Errorf("replica %s runtime: %w", channel, runtimeErr))
			stopCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			_, _ = c.manager.remove(stopCtx, channel, r)
		})
	}
	if err = e.StartContext(runtimeCtx); err != nil {
		cleanup()
		c.report(fmt.Errorf("replica %s start: %w", channel, err))
		return err
	}
	if !detach() || ctx.Err() != nil {
		cleanup()
		return ctx.Err()
	}
	registrationMu.Lock()
	if failed != nil {
		err = failed
	} else {
		err = c.manager.Add(channel, r)
	}
	registrationMu.Unlock()
	if err != nil {
		cleanup()
		c.report(fmt.Errorf("replica %s registration: %w", channel, err))
		return err
	}
	return nil
}

type managedReplica struct{ ManagedEngine }

type ownedReplica struct {
	managedReplica
	cancel context.CancelFunc
}

func (r *ownedReplica) Stop(ctx context.Context) error {
	r.cancel()
	return r.StopContext(ctx)
}

func (r managedReplica) Stop(ctx context.Context) error { return r.StopContext(ctx) }
func (c *ManagedReplicaController) RemoveReplica(ctx context.Context, channel string) error {
	if c == nil || c.manager == nil {
		return fmt.Errorf("replica controller is not configured")
	}
	_, err := c.manager.Remove(ctx, strings.TrimSpace(channel))
	return err
}
func (c *ManagedReplicaController) SetPrefix(prefix string) {
	if c == nil || c.manager == nil {
		return
	}
	for _, replica := range c.manager.ManagedEngines() {
		if setter, ok := replica.(interface{ SetPrefix(string) }); ok {
			setter.SetPrefix(prefix)
		}
	}
}

func (c *ManagedReplicaController) ReplicaChannels() []string {
	if c == nil || c.manager == nil {
		return nil
	}
	return c.manager.Channels()
}
