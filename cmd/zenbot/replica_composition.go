package main

import (
	"context"

	"zenbot/internal/command"
	"zenbot/internal/core"
	"zenbot/internal/factory"
)

// newProductionReplicaConstructor registers local utilities before the
// controller starts the transport. Replicas retain their own raw capability
// surface, without a host lifecycle or credentialed snapshot binding.
func newProductionReplicaConstructor(rf factory.ReplicaFactory) core.ReplicaConstructor {
	return func(ctx context.Context, channel string) (core.ManagedEngine, error) {
		e, err := rf.NewReplica(ctx, channel)
		if err != nil {
			return nil, err
		}
		if err := command.RegisterUserUtilities(e); err != nil {
			return nil, err
		}
		return e, nil
	}
}
