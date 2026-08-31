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

func TestDefaultChainPlacesRelayBeforeAllDownstreamHandlers(t *testing.T) {
	handlers := DefaultChainWithParticipation(participationStub{}).Handlers()
	want := []reflect.Type{
		reflect.TypeOf(ResolveUserMetadata{}),
		reflect.TypeOf(AuditChatMessage{}),
		reflect.TypeOf(IgnoreBotMessage{}),
		reflect.TypeOf(RelayAgentMessage{}),
		reflect.TypeOf(LogChatMessage{}),
		reflect.TypeOf(DeliverPendingMail{}),
		reflect.TypeOf(UpdateAfkState{}),
		reflect.TypeOf(YoutubePreview{}),
		reflect.TypeOf(CernEasterEgg{}),
		reflect.TypeOf(AgentParticipation{}),
		reflect.TypeOf(DispatchUserCommand{}),
	}
	if len(handlers) != len(want) {
		t.Fatalf("handler count=%d, want %d", len(handlers), len(want))
	}
	for i, wantType := range want {
		if gotType := reflect.TypeOf(handlers[i]); gotType != wantType {
			t.Fatalf("handler[%d]=%v, want %v", i, gotType, wantType)
		}
	}
	if _, ok := handlers[9].(AgentParticipation).Participation.(participationStub); !ok {
		t.Fatal("Slice 1 participation was not retained in the default chain")
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

type participationStub struct {
	claimed bool
	err     error
}

func (s participationStub) Handle(context.Context, *Context) (bool, error) { return s.claimed, s.err }

func TestAgentParticipationMapsPassClaimAndError(t *testing.T) {
	ctx := &Context{}
	for _, tc := range []struct {
		name, want string
		stub       participationStub
	}{
		{name: "pass", want: "next", stub: participationStub{}},
		{name: "claimed", want: "stop", stub: participationStub{claimed: true}},
		{name: "error", want: "error", stub: participationStub{claimed: true, err: errors.New("busy")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next, err := (AgentParticipation{Participation: tc.stub}).Handle(context.Background(), ctx)
			switch tc.want {
			case "next":
				if !next || err != nil {
					t.Fatalf("next=%v err=%v", next, err)
				}
			case "stop":
				if next || err != nil {
					t.Fatalf("next=%v err=%v", next, err)
				}
			case "error":
				if next || !errors.Is(err, tc.stub.err) {
					t.Fatalf("next=%v err=%v", next, err)
				}
			}
		})
	}
}

func TestAgentParticipationPassContinuesListenerChain(t *testing.T) {
	calls := 0
	c := NewChain(
		AgentParticipation{Participation: participationStub{}},
		HandlerFunc(func(context.Context, *Context) (bool, error) {
			calls++
			return true, nil
		}),
	)
	if err := c.Process(context.Background(), &model.ChatMessage{}, &fakeEngine{}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("downstream handler calls=%d, want 1", calls)
	}
}
