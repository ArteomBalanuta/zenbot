package main

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/core"
)

func newAutorunCallback(cfg *config.Config) func(common.Engine) {
	var once sync.Once
	return func(engine common.Engine) {
		if cfg == nil || engine == nil {
			return
		}
		once.Do(func() {
			for _, configured := range cfg.AutorunCommands {
				command := strings.TrimSpace(configured)
				if command == "" {
					continue
				}
				message := command
				if !strings.HasPrefix(command, "/") {
					message = "/whisper " + engine.GetName() + " " + engine.GetPrefix() + command
				}
				if _, err := engine.SendChatMessage("", message, false); err != nil {
					log.Printf("autorun command %q: %v", command, err)
				}
			}
		})
	}
}

func runHostRecovery(
	ctx context.Context,
	events <-chan error,
	policy core.RetryPolicy,
	healthy func() bool,
	restart func(context.Context) error,
	report func(error),
) {
	if ctx == nil {
		ctx = context.Background()
	}
	var ticker *time.Ticker
	var health <-chan time.Time
	if !policy.DisableHealthChecks {
		interval := policy.HealthInterval
		if interval <= 0 {
			interval = 15 * time.Second
		}
		ticker = time.NewTicker(interval)
		health = ticker.C
		defer ticker.Stop()
	}
	requestRestart := func() {
		if policy.DisableHealthChecks || restart == nil {
			return
		}
		if err := restart(ctx); err != nil && report != nil {
			report(err)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case err, open := <-events:
			if !open {
				events = nil
				continue
			}
			if err != nil && report != nil {
				report(err)
			}
			if err != nil {
				requestRestart()
			}
		case <-health:
			if healthy != nil && !healthy() {
				requestRestart()
			}
		}
	}
}
