package listener

import (
	"context"
	"testing"

	"zenbot/internal/listener/info"
)

func TestInfoChatListenerContextChildLifetime(t *testing.T) {
	var captured context.Context
	l := &InfoChatListener{chain: info.NewChain(info.HandlerFunc(func(ctx context.Context, _ *info.Context) (bool, error) {
		captured = ctx
		if ctx.Err() != nil {
			t.Fatalf("child canceled during handler: %v", ctx.Err())
		}
		return true, nil
	}))}
	contextual, ok := any(l).(interface{ NotifyContext(context.Context, string) })
	if !ok {
		t.Fatal("info listener has no contextual notification")
	}
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	contextual.NotifyContext(parent, `{}`)
	if captured == nil || captured == parent || captured.Err() != context.Canceled || parent.Err() != nil {
		t.Fatalf("incorrect child lifetime: child=%v parent error=%v", captured, parent.Err())
	}
	contextual.NotifyContext(nil, `{}`)
}
