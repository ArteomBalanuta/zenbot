package listener

import (
	"context"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/listener/message"
	"zenbot/internal/model"
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

type chatContextEngine struct{ common.Engine }

func (*chatContextEngine) GetActiveUsers() *map[*model.User]struct{} {
	users := map[*model.User]struct{}{}
	return &users
}
