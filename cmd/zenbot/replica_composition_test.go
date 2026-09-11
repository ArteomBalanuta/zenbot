package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"zenbot/internal/command"
	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/factory"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/transport"
)

type replicaCompositionTransport struct {
	messages chan transport.InboundMessage
	sent     chan string
}

func (tr *replicaCompositionTransport) Start(context.Context) error               { return nil }
func (tr *replicaCompositionTransport) Messages() <-chan transport.InboundMessage { return tr.messages }
func (*replicaCompositionTransport) Errors() <-chan error                         { return nil }
func (*replicaCompositionTransport) Connected() bool                              { return true }
func (tr *replicaCompositionTransport) SendText(_ context.Context, text string) error {
	tr.sent <- text
	return nil
}
func (tr *replicaCompositionTransport) SendRaw(ctx context.Context, text []byte) error {
	return tr.SendText(ctx, string(text))
}
func (*replicaCompositionTransport) Close(context.Context) error { return nil }

func TestProductionReplicaRespondsToLocalCommandAfterRequestEnds(t *testing.T) {
	cfg := &config.Config{Channel: "host", Name: "bot", CmdPrefix: "!"}
	rf := factory.ReplicaFactory{Config: cfg, Repository: &repository.DummyImpl{}}
	// This is the production constructor; registration must precede startup.
	construct := newProductionReplicaConstructor(rf)
	tr := &replicaCompositionTransport{messages: make(chan transport.InboundMessage, 1), sent: make(chan string, 2)}
	m := core.NewReplicaManager("host")
	c := core.NewManagedReplicaController(m, func(ctx context.Context, channel string) (core.ManagedEngine, error) {
		engine, err := construct(ctx, channel)
		if err != nil {
			return nil, err
		}
		e := engine.(*core.EngineImpl)
		e.Transport = tr
		e.AddActiveUser(&model.User{Name: "alice", Trip: "trip", Hash: "hash"})
		return e, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.AddReplica(ctx, "room"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.StopAll(context.Background()) })
	cancel()
	<-tr.sent // websocket join
	tr.messages <- transport.InboundMessage{Payload: []byte(`{"cmd":"chat","nick":"alice","trip":"trip","text":"!say hello room"}`)}
	select {
	case payload := <-tr.sent:
		var response struct {
			Command string `json:"cmd"`
			Text    string `json:"text"`
		}
		if err := json.Unmarshal([]byte(payload), &response); err != nil {
			t.Fatal(err)
		}
		if response.Command != "chat" || response.Text != "hello room" {
			t.Fatalf("response=%s", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("production replica joined but did not respond to local command")
	}
}

func TestReplicaRegistrationOmitsUnavailableControllers(t *testing.T) {
	cfg := &config.Config{Channel: "room", Name: "bot", CmdPrefix: "!"}
	e, err := factory.NewEngineWithOptions(model.REPLICA, cfg, &repository.DummyImpl{}, factory.EngineOptions{AutoMoveState: core.NewAutoMoveState()})
	if err != nil {
		t.Fatal(err)
	}
	if err := command.RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"replica", "bot", "agent", "replicaoff", "offline", "botoff", "agentoff", "replicastatus", "status", "ws", "wsay", "wsa", "wsayanon", "anonsay", "automove", "restart", "shutdown", "nuke", "resurrect"} {
		if _, exists := e.EnabledCommands[alias]; exists {
			t.Errorf("unconfigured command %q registered", alias)
		}
	}
	for _, alias := range []string{"say", "prefix"} {
		if _, exists := e.EnabledCommands[alias]; !exists {
			t.Errorf("local command %q missing", alias)
		}
	}
}
