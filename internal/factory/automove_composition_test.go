package factory

import (
	"context"
	"strings"
	"testing"

	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

type autoMoveCompositionRepository struct{ repository.DummyImpl }

func (autoMoveCompositionRepository) UserTrips(context.Context) ([]string, error) {
	return []string{"allowed-trip"}, nil
}

// TestNewEngineWithOptionsSharesAutoMoveStateAndComposesPermanentReplicaJoinPolicy
// protects the process-wide automove boundary: host and permanent replicas share
// one state, while temporary snapshot engines remain deliberately uncomposed.
func TestNewEngineWithOptionsSharesAutoMoveStateAndComposesPermanentReplicaJoinPolicy(t *testing.T) {
	state := core.NewAutoMoveState()
	if _, err := state.Configure("purgatory", "destination"); err != nil {
		t.Fatal(err)
	}
	state.SetEnabled(true)
	cfg := &config.Config{Channel: "master", Name: "bot", CmdPrefix: "!"}
	opts := EngineOptions{AutoMoveState: state}

	master, err := NewEngineWithOptions(model.MASTER, cfg, &autoMoveCompositionRepository{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got := master.AutoMoveState(); got != state {
		t.Fatalf("master automove state = %p, want shared %p", got, state)
	}

	replicas := ReplicaFactory{Config: cfg, Repository: &autoMoveCompositionRepository{}, Options: opts}
	replica, err := replicas.NewReplica(nil, "purgatory")
	if err != nil {
		t.Fatal(err)
	}
	if got := replica.AutoMoveState(); got != state {
		t.Fatalf("permanent replica automove state = %p, want shared %p", got, state)
	}
	replica.Transport = nil
	replica.UserJoinedListener.Notify(`{"nick":"joined","trip":"allowed-trip","hash":"hash"}`)
	first, second := <-replica.OutMessageQueue, <-replica.OutMessageQueue
	if !strings.Contains(first, "your trip is authorized to join ?lounge") || !strings.Contains(second, `"cmd":"kick"`) || !strings.Contains(second, `"to":"destination"`) {
		t.Fatalf("permanent replica automove outputs = %q, %q", first, second)
	}

	temporary, err := NewEngineWithOptions(model.REPLICA, &config.Config{Channel: "snapshot", Name: "bot"}, &repository.DummyImpl{}, EngineOptions{ListenerProfile: core.TemporaryOnlineSet, AutoMoveState: state})
	if err != nil {
		t.Fatal(err)
	}
	if got := temporary.AutoMoveState(); got != nil {
		t.Fatalf("temporary snapshot automove state = %p, want nil", got)
	}
	if temporary.UserJoinedListener == nil {
		t.Fatal("temporary snapshot engine lost dummy join listener")
	}
}
