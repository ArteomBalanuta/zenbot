package core

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"zenbot/internal/listener"
	"zenbot/internal/listener/message"
	"zenbot/internal/profiling"
	"zenbot/internal/transport"
)

type contextualChatNotifier struct {
	notifyCalls        int
	notifyContextCalls int
	ctx                context.Context
}

func (n *contextualChatNotifier) Notify(string) { n.notifyCalls++ }
func (n *contextualChatNotifier) NotifyContext(ctx context.Context, _ string) {
	n.notifyContextCalls++
	n.ctx = ctx
}

type plainChatNotifier struct{ calls int }

func (n *plainChatNotifier) Notify(string) { n.calls++ }

func TestDispatchMessageContextPassesLifecycleContextOnlyToOptionalChatNotifier(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	notifier := &contextualChatNotifier{}
	engine := &EngineImpl{UserChatListener: notifier}

	engine.DispatchMessageContext(cancelled, `{"cmd":"chat","nick":"alice","text":"!l question"}`)

	if notifier.notifyCalls != 0 || notifier.notifyContextCalls != 1 {
		t.Fatalf("notify=%d notifyContext=%d, want 0/1", notifier.notifyCalls, notifier.notifyContextCalls)
	}
	if notifier.ctx != cancelled || notifier.ctx.Err() == nil {
		t.Fatalf("notifier context=%#v, want cancelled lifecycle context", notifier.ctx)
	}
}

func TestDispatchMessageRetainsBackgroundCompatibilityForContextualChatNotifier(t *testing.T) {
	notifier := &contextualChatNotifier{}
	engine := &EngineImpl{UserChatListener: notifier}

	engine.DispatchMessage(`{"cmd":"chat","nick":"alice","text":"!l question"}`)

	if notifier.notifyCalls != 0 || notifier.notifyContextCalls != 1 {
		t.Fatalf("notify=%d notifyContext=%d, want 0/1", notifier.notifyCalls, notifier.notifyContextCalls)
	}
	if notifier.ctx == nil || notifier.ctx.Err() != nil {
		t.Fatalf("notifier context err=%v, want background compatibility", notifier.ctx.Err())
	}
}

func TestDispatchMessageContextFallsBackToLegacyChatNotifier(t *testing.T) {
	notifier := &plainChatNotifier{}
	engine := &EngineImpl{UserChatListener: notifier}

	engine.DispatchMessageContext(context.Background(), `{"cmd":"chat","nick":"alice","text":"hello"}`)

	if notifier.calls != 1 {
		t.Fatalf("legacy Notify calls=%d, want 1", notifier.calls)
	}
}

type lifecycleTransport struct {
	messages chan transport.InboundMessage
	errors   chan error
	mu       sync.Mutex
	started  bool
	closed   bool
}

func (t *lifecycleTransport) Start(context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.started = true
	return nil
}
func (t *lifecycleTransport) Messages() <-chan transport.InboundMessage { return t.messages }
func (t *lifecycleTransport) Errors() <-chan error                      { return t.errors }
func (t *lifecycleTransport) Connected() bool                           { return true }
func (t *lifecycleTransport) SendText(context.Context, string) error {
	return nil
}
func (t *lifecycleTransport) SendRaw(context.Context, []byte) error { return nil }
func (t *lifecycleTransport) Close(context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

type blockingContextualChatNotifier struct {
	started   chan struct{}
	cancelled chan struct{}
	once      sync.Once
}

func (*blockingContextualChatNotifier) Notify(string) {}
func (n *blockingContextualChatNotifier) NotifyContext(ctx context.Context, _ string) {
	n.once.Do(func() { close(n.started) })
	<-ctx.Done()
	close(n.cancelled)
}

func TestStartContextCancelsSynchronousChatDispatchOnLifecycleStop(t *testing.T) {
	transport := &lifecycleTransport{messages: make(chan transport.InboundMessage, 1), errors: make(chan error)}
	notifier := &blockingContextualChatNotifier{started: make(chan struct{}), cancelled: make(chan struct{})}
	engine := &EngineImpl{Channel: "room", Name: "bot", Transport: transport, UserChatListener: notifier}
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := engine.StartContext(parent); err != nil {
		t.Fatal(err)
	}
	transport.messages <- transportpkgMessage(`{"cmd":"chat","nick":"alice","text":"!l question"}`)
	select {
	case <-notifier.started:
	case <-time.After(time.Second):
		t.Fatal("chat dispatch did not reach contextual notifier")
	}
	cancel()
	select {
	case <-notifier.cancelled:
	case <-time.After(time.Second):
		t.Fatal("lifecycle cancellation did not reach synchronous chat dispatch")
	}
	if err := engine.StopContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if !transport.started || !transport.closed {
		t.Fatalf("transport started=%v closed=%v", transport.started, transport.closed)
	}
}

func transportpkgMessage(payload string) transport.InboundMessage {
	return transport.InboundMessage{Payload: []byte(payload), ReceivedAt: time.Now()}
}

type headOfLineHandler struct {
	firstStarted chan struct{}
	releaseFirst chan struct{}
	secondSeen   chan struct{}
}

func (h *headOfLineHandler) Handle(_ context.Context, state *message.Context) (bool, error) {
	switch {
	case strings.Contains(state.Message.Text, "first"):
		close(h.firstStarted)
		<-h.releaseFirst
	case strings.Contains(state.Message.Text, "second"):
		close(h.secondSeen)
	}
	return false, nil
}

type headOfLineProfileHandler struct {
	mu      sync.Mutex
	records []map[string]any
}

func (*headOfLineProfileHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *headOfLineProfileHandler) Handle(_ context.Context, record slog.Record) error {
	if record.Message != "command.profile.started" {
		return nil
	}
	attributes := map[string]any{"event": record.Message}
	record.Attrs(func(attribute slog.Attr) bool {
		attributes[attribute.Key] = attribute.Value.Any()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, attributes)
	h.mu.Unlock()
	return nil
}
func (h *headOfLineProfileHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *headOfLineProfileHandler) WithGroup(string) slog.Handler      { return h }

func TestRegularCommandProfilerExposesSynchronousInboundHeadOfLineDelay(t *testing.T) {
	transportStub := &lifecycleTransport{messages: make(chan transport.InboundMessage, 2), errors: make(chan error)}
	logHandler := &headOfLineProfileHandler{}
	profiler := profiling.New(profiling.Settings{
		Enabled:              true,
		SlowCommandThreshold: time.Millisecond,
		SlowStageThreshold:   time.Hour,
	}, slog.New(logHandler))
	blocking := &headOfLineHandler{
		firstStarted: make(chan struct{}),
		releaseFirst: make(chan struct{}),
		secondSeen:   make(chan struct{}),
	}
	engine := &EngineImpl{
		Channel:         "programming",
		Name:            "bot",
		Prefix:          "*",
		Transport:       transportStub,
		CommandProfiler: profiler,
	}
	engine.UserChatListener = listener.NewUserChatListenerWithChain(engine, message.NewChain(blocking))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := engine.StartContext(ctx); err != nil {
		t.Fatal(err)
	}
	transportStub.messages <- transport.InboundMessage{
		Payload:    []byte(`{"cmd":"chat","nick":"alice","text":"*ping first"}`),
		ReceivedAt: time.Now(),
	}
	select {
	case <-blocking.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first command did not start")
	}
	transportStub.messages <- transport.InboundMessage{
		Payload:    []byte(`{"cmd":"chat","nick":"alice","text":"*ping second"}`),
		ReceivedAt: time.Now(),
	}
	select {
	case <-blocking.secondSeen:
		t.Fatal("second command bypassed the blocked synchronous handler")
	case <-time.After(30 * time.Millisecond):
	}
	close(blocking.releaseFirst)
	select {
	case <-blocking.secondSeen:
	case <-time.After(time.Second):
		t.Fatal("second command did not resume after the first handler completed")
	}
	cancel()
	if err := engine.StopContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	logHandler.mu.Lock()
	defer logHandler.mu.Unlock()
	if len(logHandler.records) != 2 {
		t.Fatalf("started records=%#v, want two commands", logHandler.records)
	}
	queueDelay, ok := logHandler.records[1]["ws_queue_ms"].(float64)
	if !ok || queueDelay < 25 {
		t.Fatalf("second ws_queue_ms=%v, want head-of-line delay >=25ms", logHandler.records[1]["ws_queue_ms"])
	}
}
