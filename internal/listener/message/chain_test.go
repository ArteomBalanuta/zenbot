package message

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type fakeEngine struct{ common.Engine }

func TestDefaultChainOrder(t *testing.T) {
	got := []string{}
	names := []string{"ResolveUserMetadata", "AuditChatMessage", "IgnoreBotMessage", "RelayAgentMessage", "LogChatMessage", "DeliverPendingMail", "UpdateAfkState", "YoutubePreview", "CernEasterEgg", "AgentParticipation", "DispatchUserCommand"}
	for _, n := range names {
		got = append(got, n)
	}
	if !reflect.DeepEqual(got, names) {
		t.Fatal(got)
	}
}
func TestChainShortCircuitAndErrors(t *testing.T) {
	calls := []string{}
	c := NewChain(HandlerFunc(func(context.Context, *Context) (bool, error) { calls = append(calls, "a"); return false, nil }), HandlerFunc(func(context.Context, *Context) (bool, error) { calls = append(calls, "b"); return true, nil }))
	if err := c.Process(context.Background(), &model.ChatMessage{}, &fakeEngine{}); err != nil || len(calls) != 1 {
		t.Fatalf("%v %v", err, calls)
	}
	want := errors.New("stop")
	c = NewChain(HandlerFunc(func(context.Context, *Context) (bool, error) { return true, want }), HandlerFunc(func(context.Context, *Context) (bool, error) { t.Fatal("called"); return true, nil }))
	if !errors.Is(c.Process(context.Background(), &model.ChatMessage{}, &fakeEngine{}), want) {
		t.Fatal("error not preserved")
	}
}
