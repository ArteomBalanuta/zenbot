package factory

import (
	"context"
	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

type ReplicaFactory struct {
	Config     *config.Config
	Repository repository.Repository
	Options    EngineOptions
}

func (f ReplicaFactory) NewReplica(ctx context.Context, channel string) (*core.EngineImpl, error) {
	c := *f.Config
	c.Channel = channel
	c.Name = f.Config.Name
	return NewEngineWithOptions(model.REPLICA, &c, f.Repository, f.Options)
}
