package live

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/agent/moderation"
	"zenbot/internal/agent/participation"
	"zenbot/internal/model"
)

type monitorStub struct {
	decisions []moderation.Decision
	seen      int
}

func (m *monitorStub) OnMessage(model.ChatMessage) []moderation.Decision {
	m.seen++
	return append([]moderation.Decision(nil), m.decisions...)
}

type actionStub struct {
	calls int
	err   error
}

func (x *actionStub) Execute(context.Context, moderation.Decision) error { x.calls++; return x.err }

func TestMessageAutomationObservesPrefixBeforePipelineFilteringAndContainsActionError(t *testing.T) {
	monitor := &monitorStub{decisions: []moderation.Decision{{Action: moderation.Warn, Principal: "alice"}}}
	executor := &actionStub{err: errors.New("blocked")}
	a := &MessageAutomation{Monitor: monitor, Executor: executor}
	pipeline := &participation.Pipeline{Monitor: func(e participation.Event) { a.Observe(context.Background(), e) }}
	out := pipeline.Handle(participation.Event{Message: model.ChatMessage{Name: "alice", Text: "!help"}, Prefix: "!"})
	if out.Decision != participation.Pass || monitor.seen != 1 || executor.calls != 1 {
		t.Fatalf("out=%+v monitor=%d executor=%d", out, monitor.seen, executor.calls)
	}
}
