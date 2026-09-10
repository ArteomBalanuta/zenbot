package listener

import (
	"context"
	"log/slog"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	"zenbot/internal/common"
	"zenbot/internal/listener/message"
	"zenbot/internal/model"
	"zenbot/internal/profiling"
)

type observedChatContext struct {
	ctx       context.Context
	errDuring error
	calls     int
}

func (h *observedChatContext) Handle(ctx context.Context, _ *message.Context) (bool, error) {
	h.ctx = ctx
	h.errDuring = ctx.Err()
	h.calls++
	return false, nil
}

func TestUserChatListenerNotifyContextUsesChildOnlyForSynchronousChain(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	observed := &observedChatContext{}
	listener := NewUserChatListenerWithChain(&chatContextEngine{}, message.NewChain(observed))

	listener.NotifyContext(parent, `{"cmd":"chat","nick":"alice","text":"hello"}`)

	if observed.calls != 1 {
		t.Fatalf("handler calls=%d, want 1", observed.calls)
	}
	if observed.ctx == nil || observed.ctx == parent {
		t.Fatalf("handler context=%#v, want distinct child context", observed.ctx)
	}
	if observed.errDuring != nil {
		t.Fatalf("child context ended during synchronous processing: %v", observed.errDuring)
	}
	if err := observed.ctx.Err(); err == nil {
		t.Fatal("child context remained live after NotifyContext returned")
	}
}

func TestUserChatListenerNotifyRetainsBackgroundCompatibility(t *testing.T) {
	observed := &observedChatContext{}
	listener := NewUserChatListenerWithChain(&chatContextEngine{}, message.NewChain(observed))

	listener.Notify(`{"cmd":"chat","nick":"alice","text":"hello"}`)

	if observed.calls != 1 {
		t.Fatalf("handler calls=%d, want 1", observed.calls)
	}
	if observed.errDuring != nil {
		t.Fatalf("Notify handler context err=%v, want live background-derived child", observed.errDuring)
	}
	if observed.ctx == nil || observed.ctx.Err() == nil {
		t.Fatal("Notify child context remained live after synchronous processing")
	}
}

type chatContextEngine struct {
	common.Engine
	profiler *profiling.Profiler
}

func (*chatContextEngine) GetActiveUsers() *map[*model.User]struct{} {
	users := map[*model.User]struct{}{}
	return &users
}
func (*chatContextEngine) GetPrefix() string  { return "*" }
func (*chatContextEngine) GetChannel() string { return "programming" }
func (e *chatContextEngine) PerformanceProfiler() *profiling.Profiler {
	return e.profiler
}

type profilingEventHandler struct {
	mu     sync.Mutex
	events []string
}

func (*profilingEventHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *profilingEventHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	h.events = append(h.events, record.Message)
	h.mu.Unlock()
	return nil
}
func (h *profilingEventHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *profilingEventHandler) WithGroup(string) slog.Handler      { return h }

func TestUserChatListenerProfilesRegularCommandChainButExcludesAgentCommand(t *testing.T) {
	handler := &profilingEventHandler{}
	profiler := profiling.New(profiling.Settings{
		Enabled:              true,
		SlowCommandThreshold: time.Millisecond,
		SlowStageThreshold:   time.Millisecond,
	}, slog.New(handler))
	engine := &chatContextEngine{profiler: profiler}
	var commandLabels []string
	chain := message.NewChain(message.HandlerFunc(func(ctx context.Context, _ *message.Context) (bool, error) {
		label, _ := pprof.Label(ctx, "zenbot.command")
		commandLabels = append(commandLabels, label)
		time.Sleep(2 * time.Millisecond)
		return false, nil
	}))
	listener := NewUserChatListenerWithChain(engine, chain)

	listener.NotifyContext(context.Background(), `{"cmd":"chat","nick":"alice","text":"*ping"}`)
	listener.NotifyContext(context.Background(), `{"cmd":"chat","nick":"alice","text":"*l explain"}`)

	handler.mu.Lock()
	defer handler.mu.Unlock()
	want := []string{"command.profile.started", "command.profile.stage", "command.profile.completed"}
	if len(handler.events) != len(want) {
		t.Fatalf("profiling events=%v, want %v", handler.events, want)
	}
	for index := range want {
		if handler.events[index] != want[index] {
			t.Fatalf("profiling events=%v, want %v", handler.events, want)
		}
	}
	if len(commandLabels) != 2 || commandLabels[0] != "ping" || commandLabels[1] != "" {
		t.Fatalf("pprof command labels=%v, want [ping <empty>]", commandLabels)
	}
}
