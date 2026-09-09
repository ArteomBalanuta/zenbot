package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/core"
)

type autorunTestEngine struct {
	common.Engine
	messages []string
}

func (e *autorunTestEngine) GetName() string   { return "korin" }
func (e *autorunTestEngine) GetPrefix() string { return "*" }
func (e *autorunTestEngine) SendChatMessage(_ string, message string, _ bool) (string, error) {
	e.messages = append(e.messages, message)
	return message, nil
}

func TestAutorunCallbackExecutesSaturnStartupCommandsOnce(t *testing.T) {
	engine := &autorunTestEngine{}
	callback := newAutorunCallback(&config.Config{AutorunCommands: []string{"replica lounge", " /move korin lounge "}})
	callback(engine)
	callback(engine)
	want := []string{"/whisper korin *replica lounge", "/move korin lounge"}
	if len(engine.messages) != len(want) {
		t.Fatalf("messages=%q", engine.messages)
	}
	for index := range want {
		if engine.messages[index] != want[index] {
			t.Fatalf("messages=%q, want %q", engine.messages, want)
		}
	}
}

func TestHostRecoveryRestartsForTransportFailureAndFailedHealth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan error, 1)
	var healthy atomic.Bool
	healthy.Store(true)
	restarts := make(chan struct{}, 2)
	go runHostRecovery(ctx, events, core.RetryPolicy{HealthInterval: time.Millisecond}, healthy.Load, func(context.Context) error {
		restarts <- struct{}{}
		return nil
	}, nil)

	events <- errors.New("transport closed")
	waitForRestart(t, restarts)
	healthy.Store(false)
	waitForRestart(t, restarts)
}

func TestHostRecoveryDoesNotRestartWhenAutoReconnectIsDisabled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan error, 1)
	var restarts atomic.Int32
	go runHostRecovery(ctx, events, core.RetryPolicy{DisableHealthChecks: true}, func() bool { return false }, func(context.Context) error {
		restarts.Add(1)
		return nil
	}, nil)
	events <- errors.New("transport closed")
	time.Sleep(10 * time.Millisecond)
	cancel()
	if restarts.Load() != 0 {
		t.Fatalf("restarts=%d", restarts.Load())
	}
}

func waitForRestart(t *testing.T, restarts <-chan struct{}) {
	t.Helper()
	select {
	case <-restarts:
	case <-time.After(time.Second):
		t.Fatal("restart was not requested")
	}
}
