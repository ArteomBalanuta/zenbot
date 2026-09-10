package core

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestAutoMoveDefaultsAreFalsePurgatoryAndLounge(t *testing.T) {
	s := NewAutoMoveState()
	got := s.Snapshot()
	if got.Enabled || got.Destination != "lounge" || len(got.Sources) != 1 || got.Sources[0] != "purgatory" {
		t.Fatalf("snapshot=%+v", got)
	}
	if _, ok := s.EligibleReplica("Purgatory"); ok {
		t.Fatal("source membership must be case-sensitive")
	}
}

func TestAutoMoveStateConfigure(t *testing.T) {
	s := NewAutoMoveState()
	before := s.Snapshot()

	got, err := s.Configure("  hell  ", "  heaven  ")
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	wantSources := []string{"hell", "purgatory"}
	if !reflect.DeepEqual(got.Sources, wantSources) || got.Destination != "heaven" || got.Enabled {
		t.Fatalf("Configure() snapshot = %+v, want disabled/%v/heaven", got, wantSources)
	}
	if !reflect.DeepEqual(before.Sources, []string{"purgatory"}) || before.Destination != "lounge" || before.Enabled {
		t.Fatalf("previous snapshot mutated = %+v", before)
	}

	got.Sources[0] = "changed"
	if after := s.Snapshot(); !reflect.DeepEqual(after.Sources, wantSources) {
		t.Fatalf("Configure() returned mutable state snapshot; later snapshot = %+v", after)
	}
	if _, err := s.Configure("   ", "elsewhere"); err == nil {
		t.Fatal("Configure() blank source error = nil")
	}
	if _, err := s.Configure("elsewhere", "\t"); err == nil {
		t.Fatal("Configure() blank destination error = nil")
	}
	if after := s.Snapshot(); !reflect.DeepEqual(after.Sources, wantSources) || after.Destination != "heaven" {
		t.Fatalf("failed Configure() mutated state: %+v", after)
	}
}

func TestEngineEnableAutoMoveMarksStateBeforeAddingOnlyMissingSources(t *testing.T) {
	state := NewAutoMoveState()
	if _, err := state.Configure("source", "destination"); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	manager := NewReplicaManager("host")
	if err := manager.Add("purgatory", autoMoveFixtureReplica{}); err != nil {
		t.Fatalf("manager.Add(purgatory) error = %v", err)
	}
	engine := &EngineImpl{Channel: "host", autoMove: state}
	entered := make(chan string, 1)
	release := make(chan struct{})
	engine.SetReplicaController(NewManagedReplicaController(manager, func(ctx context.Context, channel string) (ManagedEngine, error) {
		if snapshot := state.Snapshot(); !snapshot.Enabled {
			t.Fatal("state was not enabled before AddReplica")
		}
		// These snapshots must not wait on locks retained over lifecycle work.
		_ = manager.Channels()
		entered <- channel
		<-release
		return &testLifecycleEngine{}, nil
	}))

	done := make(chan error, 1)
	go func() { _, err := engine.EnableAutoMove(context.Background()); done <- err }()
	select {
	case channel := <-entered:
		if channel != "source" {
			t.Fatalf("AddReplica channel = %q, want source", channel)
		}
	case <-time.After(time.Second):
		t.Fatal("AddReplica did not run without retained state or manager locks")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("EnableAutoMove() error = %v", err)
	}
	if got := manager.Channels(); !reflect.DeepEqual(got, []string{"purgatory", "source"}) {
		t.Fatalf("replica channels = %v, want purgatory and source", got)
	}
}

func TestEngineAutoMovePreCancelledContextDoesNotMutateOrAct(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	state := NewAutoMoveState()
	manager := NewReplicaManager("host")
	actions := make(chan string, 2)
	if err := manager.Add("purgatory", autoMoveStopReplica{stopped: actions}); err != nil {
		t.Fatalf("manager.Add() error = %v", err)
	}
	engine := &EngineImpl{Channel: "host", autoMove: state}
	engine.SetReplicaController(NewManagedReplicaController(manager, func(context.Context, string) (ManagedEngine, error) {
		actions <- "add"
		return &testLifecycleEngine{}, nil
	}))

	if _, err := engine.EnableAutoMove(ctx); err == nil {
		t.Fatal("EnableAutoMove() cancelled error = nil")
	}
	if snapshot := state.Snapshot(); snapshot.Enabled {
		t.Fatalf("cancelled enable mutated state: %+v", snapshot)
	}
	state.SetEnabled(true)
	if _, err := engine.DisableAutoMove(ctx); err == nil {
		t.Fatal("DisableAutoMove() cancelled error = nil")
	}
	if snapshot := state.Snapshot(); !snapshot.Enabled {
		t.Fatalf("cancelled disable mutated state: %+v", snapshot)
	}
	select {
	case action := <-actions:
		t.Fatalf("cancelled lifecycle performed %q", action)
	default:
	}
}

func TestEngineDisableAutoMoveMarksDisabledRemovesPresentSourcesAndReturnsFirstStopError(t *testing.T) {
	state := NewAutoMoveState()
	if _, err := state.Configure("source", "destination"); err != nil {
		t.Fatalf("Configure(source) error = %v", err)
	}
	if _, err := state.Configure("absent", "destination"); err != nil {
		t.Fatalf("Configure(absent) error = %v", err)
	}
	state.SetEnabled(true)
	manager := NewReplicaManager("host")
	stops := make(chan string, 2)
	first := errors.New("first stop failed")
	if err := manager.Add("purgatory", autoMoveErrorStopReplica{channel: "purgatory", stopped: stops, err: first}); err != nil {
		t.Fatalf("manager.Add(purgatory) error = %v", err)
	}
	if err := manager.Add("source", autoMoveErrorStopReplica{channel: "source", stopped: stops}); err != nil {
		t.Fatalf("manager.Add(source) error = %v", err)
	}
	engine := &EngineImpl{Channel: "host", autoMove: state}
	engine.SetReplicaController(NewManagedReplicaController(manager, nil))

	if _, err := engine.DisableAutoMove(context.Background()); !errors.Is(err, first) {
		t.Fatalf("DisableAutoMove() error = %v, want first stop error", err)
	}
	if snapshot := state.Snapshot(); snapshot.Enabled {
		t.Fatalf("DisableAutoMove() did not mark state disabled before stops: %+v", snapshot)
	}
	gotStops := []string{<-stops, <-stops}
	if !reflect.DeepEqual(gotStops, []string{"purgatory", "source"}) {
		t.Fatalf("stops = %v, want present sources only", gotStops)
	}
	if got := manager.Channels(); len(got) != 0 {
		t.Fatalf("remaining replicas = %v, want none", got)
	}
}

func TestEngineAutoMoveConcurrentEnableDisableReconcilesToDisabledWithoutReplicas(t *testing.T) {
	state := NewAutoMoveState()
	manager := NewReplicaManager("host")
	entered := make(chan struct{})
	release := make(chan struct{})
	engine := &EngineImpl{Channel: "host", autoMove: state}
	engine.SetReplicaController(NewManagedReplicaController(manager, func(context.Context, string) (ManagedEngine, error) {
		select {
		case entered <- struct{}{}:
			<-release
		default:
		}
		return &testLifecycleEngine{}, nil
	}))

	enabled := make(chan error, 1)
	go func() { _, err := engine.EnableAutoMove(context.Background()); enabled <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("enable did not reach replica lifecycle")
	}
	disabled := make(chan error, 1)
	go func() { _, err := engine.DisableAutoMove(context.Background()); disabled <- err }()
	select {
	case err := <-disabled:
		t.Fatalf("disable completed before blocked enable reconciliation: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-enabled; err != nil {
		t.Fatalf("EnableAutoMove() error = %v", err)
	}
	if err := <-disabled; err != nil {
		t.Fatalf("DisableAutoMove() error = %v", err)
	}
	if snapshot := state.Snapshot(); snapshot.Enabled {
		t.Fatalf("final snapshot = %+v, want disabled", snapshot)
	}
	if got := manager.Channels(); len(got) != 0 {
		t.Fatalf("final replicas = %v, want none", got)
	}
}

type testLifecycleEngine struct{ EngineImpl }

func (*testLifecycleEngine) StartContext(context.Context) error { return nil }
func (*testLifecycleEngine) StopContext(context.Context) error  { return nil }

type autoMoveFixtureReplica struct{}

func (autoMoveFixtureReplica) Stop(context.Context) error { return nil }

type autoMoveStopReplica struct{ stopped chan<- string }

func (r autoMoveStopReplica) Stop(context.Context) error {
	r.stopped <- "remove"
	return nil
}

type autoMoveErrorStopReplica struct {
	channel string
	stopped chan<- string
	err     error
}

func (r autoMoveErrorStopReplica) Stop(context.Context) error {
	r.stopped <- r.channel
	return r.err
}
