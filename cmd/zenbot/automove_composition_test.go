package main

import (
	"testing"

	"zenbot/internal/command"
	"zenbot/internal/config"
	"zenbot/internal/factory"
	"zenbot/internal/model"
	"zenbot/internal/profiling"
	"zenbot/internal/repository"
)

func TestNewAutoMoveProductionOptionsSharesOneStateAndRegistersController(t *testing.T) {
	cfg := &config.Config{Channel: "master", Name: "bot", CmdPrefix: "!"}
	profiler := profiling.New(profiling.Settings{Enabled: true}, nil)
	state, masterOptions, replicaOptions := newAutoMoveProductionOptions(cfg, factory.EngineOptions{Profiler: profiler})
	if state == nil || masterOptions.AutoMoveState != state || replicaOptions.AutoMoveState != state {
		t.Fatalf("automove state was not shared: state=%p master=%p replica=%p", state, masterOptions.AutoMoveState, replicaOptions.AutoMoveState)
	}
	if replicaOptions.Profiler != profiler || replicaOptions.Transport.Profiler != profiler {
		t.Fatal("replica options did not retain the process profiler")
	}

	master, err := factory.NewEngineWithOptions(model.MASTER, cfg, &repository.DummyImpl{}, masterOptions)
	if err != nil {
		t.Fatal(err)
	}
	if err := command.RegisterUserUtilitiesWithDirectAgent(master, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := master.EnabledCommands["automove"]; !ok {
		t.Fatal("fully composed master did not register automove controller command")
	}
}
