package core

import (
	"context"
	"sync"
	"testing"
	"time"
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
	messages chan []byte
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
func (t *lifecycleTransport) Messages() <-chan []byte { return t.messages }
func (t *lifecycleTransport) Errors() <-chan error    { return t.errors }
func (t *lifecycleTransport) Connected() bool         { return true }
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
	transport := &lifecycleTransport{messages: make(chan []byte, 1), errors: make(chan error)}
	notifier := &blockingContextualChatNotifier{started: make(chan struct{}), cancelled: make(chan struct{})}
	engine := &EngineImpl{Channel: "room", Name: "bot", Transport: transport, UserChatListener: notifier}
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := engine.StartContext(parent); err != nil {
		t.Fatal(err)
	}
	transport.messages <- []byte(`{"cmd":"chat","nick":"alice","text":"!l question"}`)
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
