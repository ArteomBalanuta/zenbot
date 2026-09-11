package core

import (
	"sync"
	"testing"
	"time"
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

func TestFinalIntegrationCompatibilitySetPrefixAndReadAreRaceFree(t *testing.T) {
	host := &EngineImpl{Prefix: "!"}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				host.SetPrefix("$")
				_ = host.GetPrefix()
			}
		}()
	}
	wg.Wait()
}

type prefixCallbackEngine struct {
	ManagedEngine
	host     *EngineImpl
	observed chan string
}

func (e prefixCallbackEngine) SetPrefix(string) { e.observed <- e.host.GetPrefix() }

func TestFinalIntegrationCompatibilityPrefixUnlocksBeforePropagation(t *testing.T) {
	host := &EngineImpl{Prefix: "!"}
	callback := prefixCallbackEngine{host: host, observed: make(chan string, 1)}
	manager := NewReplicaManager("host")
	if err := manager.Add("replica", managedReplica{ManagedEngine: callback}); err != nil {
		t.Fatal(err)
	}
	host.SetReplicaController(NewManagedReplicaController(manager, nil))
	done := make(chan struct{})
	go func() { host.SetPrefix("$"); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("prefix propagation held host lock across callback")
	}
	if got := <-callback.observed; got != "$" {
		t.Fatalf("callback observed prefix=%q", got)
	}
}
