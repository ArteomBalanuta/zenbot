package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

func TestSupportRelayRendersEscapesAndSendsOnlyToManagedSupport(t *testing.T) {
	manager := NewReplicaManager("host")
	support := &EngineImpl{Channel: "support", OutMessageQueue: make(chan string, 1)}
	if err := manager.Add("support", managedReplica{ManagedEngine: support}); err != nil {
		t.Fatal(err)
	}
	relay := NewSupportReplicaRelay(manager, []string{"admin-trip"})
	if err := relay.RelayToSupport(context.Background(), common.SupportRelayRequest{Author: "alice", Trip: "admin-trip", Arguments: []string{"say!", `"\\`, "line\n\t\x01"}}); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(<-support.OutMessageQueue), &payload); err != nil {
		t.Fatal(err)
	}
	if want := "alice: say! \"\\\\ line\n\t\u0001 "; payload.Text != want {
		t.Fatalf("text=%q, want %q", payload.Text, want)
	}
	if err := relay.RelayToSupport(context.Background(), common.SupportRelayRequest{Author: "alice", Trip: "other", Anonymous: true, Arguments: []string{"say!", "<tag>", "😀"}}); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(<-support.OutMessageQueue), &payload); err != nil {
		t.Fatal(err)
	}
	if want := "anon_from_hc: say tag  "; payload.Text != want {
		t.Fatalf("anonymous text=%q, want %q", payload.Text, want)
	}
	if channels := manager.Channels(); len(channels) != 1 || channels[0] != "support" {
		t.Fatalf("relay mutated replicas: %v", channels)
	}
}

func TestSupportRelayFailsClosedWithoutSupportOnSendFailureOrCancellation(t *testing.T) {
	manager := NewReplicaManager("host")
	relay := NewSupportReplicaRelay(manager, nil)
	request := common.SupportRelayRequest{Author: "alice", Arguments: []string{"hello"}}
	if err := relay.RelayToSupport(context.Background(), request); err == nil {
		t.Fatal("missing support succeeded")
	}
	sendFailure := errors.New("support send failed")
	failing := &failingManagedSupport{EngineImpl: &EngineImpl{Channel: "support"}, err: sendFailure}
	if err := manager.Add("support", managedReplica{ManagedEngine: failing}); err != nil {
		t.Fatal(err)
	}
	if err := relay.RelayToSupport(context.Background(), request); !errors.Is(err, sendFailure) || failing.calls != 1 {
		t.Fatalf("send failure=%v calls=%d", err, failing.calls)
	}
	if _, err := manager.Remove(context.Background(), "support"); err != nil {
		t.Fatal(err)
	}
	support := &EngineImpl{Channel: "support", OutMessageQueue: make(chan string)}
	if err := manager.Add("support", managedReplica{ManagedEngine: support}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := relay.RelayToSupport(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled err=%v", err)
	}
	if len(support.OutMessageQueue) != 0 {
		t.Fatal("cancelled relay sent")
	}
}

type failingManagedSupport struct {
	*EngineImpl
	err   error
	calls int
}

func (e *failingManagedSupport) SendChatMessage(string, string, bool) (string, error) {
	e.calls++
	return "", e.err
}

func TestEngineSupportRelayDelegatesOnlyWhenInstalled(t *testing.T) {
	engine := &EngineImpl{}
	if err := engine.RelayToSupport(context.Background(), common.SupportRelayRequest{}); err == nil {
		t.Fatal("uninstalled relay succeeded")
	}
	manager := NewReplicaManager("host")
	support := &EngineImpl{Channel: "support", OutMessageQueue: make(chan string, 1)}
	if err := manager.Add("support", managedReplica{ManagedEngine: support}); err != nil {
		t.Fatal(err)
	}
	engine.SetSupportReplicaRelay(NewSupportReplicaRelay(manager, []string{"admin"}))
	if err := engine.RelayToSupport(context.Background(), common.SupportRelayRequest{Author: "alice", Trip: "admin", Arguments: []string{"hello"}}); err != nil {
		t.Fatal(err)
	}
	if len(support.OutMessageQueue) != 1 {
		t.Fatal("installed relay did not use support")
	}
}

var _ model.EngineType
