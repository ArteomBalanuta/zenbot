package live

import (
	"context"
	"log"
	"time"

	"zenbot/internal/agent/moderation"
	"zenbot/internal/agent/participation"
)

// MessageAutomation is the pre-filter deterministic moderation bridge. It
// observes parsed events and never changes participation routing.
type MessageAutomation struct {
	Monitor  moderation.MessageMonitor
	Executor moderation.MessageActionExecutor
	Timeout  time.Duration
}

func (a *MessageAutomation) Observe(parent context.Context, event participation.Event) {
	if a == nil || a.Monitor == nil || a.Executor == nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	timeout := a.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	for _, decision := range a.Monitor.OnMessage(event.Message) {
		ctx, cancel := context.WithTimeout(parent, timeout)
		err := a.Executor.Execute(ctx, decision)
		cancel()
		if err != nil {
			log.Printf("message moderation execution failed action=%s target=%q reason=%q: %v", decision.Action, decision.Principal, decision.Reason, err)
		}
	}
}
