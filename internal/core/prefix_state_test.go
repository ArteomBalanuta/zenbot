package core

import (
	"sync"
	"testing"
)

type opaqueManagedEngine struct{ ManagedEngine }

func TestUpdatePrefixChangesHostAndCurrentManagedReplicas(t *testing.T) {
	host := &EngineImpl{Prefix: "*"}
	replica := &EngineImpl{Prefix: "*"}
	manager := NewReplicaManager("host")
	if err := manager.Add("lounge", managedReplica{ManagedEngine: replica}); err != nil {
		t.Fatal(err)
	}
	host.SetReplicaController(NewManagedReplicaController(manager, nil))

	previous, err := host.UpdatePrefix("$")
	if err != nil {
		t.Fatal(err)
	}
	if previous != "*" || host.GetPrefix() != "$" || replica.GetPrefix() != "$" {
		t.Fatalf("previous=%q host=%q replica=%q", previous, host.GetPrefix(), replica.GetPrefix())
	}
}

func TestUpdatePrefixWithoutControllerChangesOnlyHost(t *testing.T) {
	host := &EngineImpl{Prefix: "*"}
	previous, err := host.UpdatePrefix("$")
	if err != nil {
		t.Fatal(err)
	}
	if previous != "*" || host.GetPrefix() != "$" {
		t.Fatalf("previous=%q host=%q", previous, host.GetPrefix())
	}
}

func TestUpdatePrefixUsesCurrentManagedSnapshot(t *testing.T) {
	host := &EngineImpl{Prefix: "*"}
	current := &EngineImpl{Prefix: "*"}
	future := &EngineImpl{Prefix: "*"}
	manager := NewReplicaManager("host")
	if err := manager.Add("current", managedReplica{ManagedEngine: current}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Add("opaque", managedReplica{ManagedEngine: opaqueManagedEngine{}}); err != nil {
		t.Fatal(err)
	}
	host.SetReplicaController(NewManagedReplicaController(manager, nil))
	snapshot := manager.ManagedEngines()

	if _, err := host.UpdatePrefix("$"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Add("future", managedReplica{ManagedEngine: future}); err != nil {
		t.Fatal(err)
	}
	if len(snapshot) != 2 {
		t.Fatalf("snapshot size=%d, want 2", len(snapshot))
	}
	if current.GetPrefix() != "$" || future.GetPrefix() != "*" {
		t.Fatalf("current=%q future=%q", current.GetPrefix(), future.GetPrefix())
	}
}

func TestGetPrefixAndUpdatePrefixAreRaceFree(t *testing.T) {
	host := &EngineImpl{Prefix: "*"}
	const iterations = 1_000
	var readers sync.WaitGroup
	for range 8 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for range iterations {
				_ = host.GetPrefix()
			}
		}()
	}
	for range iterations {
		if _, err := host.UpdatePrefix("$"); err != nil {
			t.Fatal(err)
		}
		if _, err := host.UpdatePrefix("*"); err != nil {
			t.Fatal(err)
		}
	}
	readers.Wait()
}
