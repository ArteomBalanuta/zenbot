package moderation

import (
	"context"
	"log"
	"time"
	"zenbot/internal/model"
)

// Automation is the trusted join-only bridge; it has no conversational runtime.
type Automation struct {
	monitor  JoinMonitor
	executor ActionExecutor
	timeout  time.Duration
}

func NewAutomation(m JoinMonitor, x ActionExecutor, timeout time.Duration) *Automation {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &Automation{monitor: m, executor: x, timeout: timeout}
}
func (a *Automation) OnJoin(parent context.Context, u *model.User) {
	if a == nil || a.monitor == nil || a.executor == nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	for _, d := range a.monitor.OnJoin(u) {
		ctx, cancel := context.WithTimeout(parent, a.timeout)
		err := a.executor.Execute(ctx, d)
		cancel()
		if err != nil {
			log.Printf("join moderation execution failed action=%s target=%q reason=%q: %v", d.Action, d.Principal, d.Reason, err)
		}
	}
}
