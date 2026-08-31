package message

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/relay"
)

type agentRelayEngine struct {
	common.Engine
	typeValue model.EngineType
	host      relay.HostRelay
}

func (e agentRelayEngine) EngineType() model.EngineType { return e.typeValue }
func (e agentRelayEngine) HostRelay() relay.HostRelay   { return e.host }

type relaySpy struct {
	calls  int
	author string
	text   string
	err    error
}

func (s *relaySpy) RelayAgentMessage(_ context.Context, author, text string) error {
	s.calls++
	s.author, s.text = author, text
	return s.err
}

func TestRelayAgentMessagePassesMasterAndReplicaWithoutHostDelivery(t *testing.T) {
	for _, engineType := range []model.EngineType{model.MASTER, model.REPLICA} {
		t.Run(engineType.String(), func(t *testing.T) {
			host := &relaySpy{}
			next, err := (RelayAgentMessage{}).Handle(context.Background(), &Context{
				Engine:  agentRelayEngine{typeValue: engineType, host: host},
				Message: &model.ChatMessage{Name: "alice", Text: "hello"},
			})
			if !next || err != nil || host.calls != 0 {
				t.Fatalf("next=%v err=%v hostCalls=%d", next, err, host.calls)
			}
		})
	}
}

func TestRelayAgentMessageRelaysAgentTextAndStopsChain(t *testing.T) {
	host := &relaySpy{}
	sentinelCalls := 0
	chain := NewChain(RelayAgentMessage{}, HandlerFunc(func(context.Context, *Context) (bool, error) {
		sentinelCalls++
		return true, nil
	}))
	err := chain.Process(context.Background(), &model.ChatMessage{Name: "alice", Text: "hello"}, agentRelayEngine{typeValue: model.AGENT, host: host})
	if err != nil {
		t.Fatal(err)
	}
	if host.calls != 1 || host.author != "alice" || host.text != "hello" || sentinelCalls != 0 {
		t.Fatalf("host=%+v sentinelCalls=%d", host, sentinelCalls)
	}
}

func TestRelayAgentMessageClaimsAgentWithoutHost(t *testing.T) {
	next, err := (RelayAgentMessage{}).Handle(context.Background(), &Context{
		Engine:  agentRelayEngine{typeValue: model.AGENT},
		Message: &model.ChatMessage{Name: "alice", Text: "hello"},
	})
	if next || err != nil {
		t.Fatalf("next=%v err=%v", next, err)
	}
}

func TestRelayAgentMessageReturnsHostErrorAndStopsChain(t *testing.T) {
	want := errors.New("transport down")
	host := &relaySpy{err: want}
	sentinelCalls := 0
	chain := NewChain(RelayAgentMessage{}, HandlerFunc(func(context.Context, *Context) (bool, error) {
		sentinelCalls++
		return true, nil
	}))
	err := chain.Process(context.Background(), &model.ChatMessage{Name: "alice", Text: "hello"}, agentRelayEngine{typeValue: model.AGENT, host: host})
	if !errors.Is(err, want) || sentinelCalls != 0 || host.calls != 1 {
		t.Fatalf("err=%v hostCalls=%d sentinelCalls=%d", err, host.calls, sentinelCalls)
	}
}
