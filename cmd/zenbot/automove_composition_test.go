package main

import (
	"testing"

	"zenbot/internal/command"
	"zenbot/internal/config"
	"zenbot/internal/factory"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

func TestNewAutoMoveProductionOptionsSharesOneStateAndRegistersController(t *testing.T) {
	cfg := &config.Config{Channel: "master", Name: "bot", CmdPrefix: "!"}
	state, masterOptions, replicaOptions := newAutoMoveProductionOptions(cfg, factory.EngineOptions{})
	if state == nil || masterOptions.AutoMoveState != state || replicaOptions.AutoMoveState != state {
		t.Fatalf("automove state was not shared: state=%p master=%p replica=%p", state, masterOptions.AutoMoveState, replicaOptions.AutoMoveState)
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
