package core

import (
	"context"
	"fmt"
	"strings"
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
	e, err := c.construct(ctx, channel)
	if err != nil {
		c.report(fmt.Errorf("replica %s construction: %w", channel, err))
		return err
	}
	if impl, ok := e.(*EngineImpl); ok {
		impl.SetRuntimeFailureHandler(func(runtimeErr error) {
			c.report(fmt.Errorf("replica %s runtime: %w", channel, runtimeErr))
			_, _ = c.manager.Remove(context.Background(), channel)
		})
	}
	if err = e.StartContext(ctx); err != nil {
		c.report(fmt.Errorf("replica %s start: %w", channel, err))
		return err
	}
	if err = c.manager.Add(channel, managedReplica{e}); err != nil {
		_ = e.StopContext(ctx)
		c.report(fmt.Errorf("replica %s registration: %w", channel, err))
		return err
	}
	return nil
}

type managedReplica struct{ ManagedEngine }

func (r managedReplica) Stop(ctx context.Context) error { return r.StopContext(ctx) }
func (c *ManagedReplicaController) RemoveReplica(ctx context.Context, channel string) error {
	if c == nil || c.manager == nil {
		return fmt.Errorf("replica controller is not configured")
	}
	_, err := c.manager.Remove(ctx, strings.TrimSpace(channel))
	return err
}
func (c *ManagedReplicaController) ReplicaChannels() []string {
	if c == nil || c.manager == nil {
		return nil
	}
	return c.manager.Channels()
}
