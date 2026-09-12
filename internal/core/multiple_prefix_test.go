package core

import (
	"reflect"
	"sync"
	"testing"
)

func TestMultiplePrefixesValidateCopyAndPropagate(t *testing.T) {
	host := &EngineImpl{Prefix: "*"}
	replica := &EngineImpl{Prefix: "*"}
	manager := NewReplicaManager("host")
	if err := manager.Add("room", managedReplica{ManagedEngine: replica}); err != nil {
		t.Fatal(err)
	}
	host.SetReplicaController(NewManagedReplicaController(manager, nil))
	controller, ok := any(host).(interface {
		UpdatePrefix(...string) (string, error)
		GetPrefixes() []string
	})
	if !ok {
		t.Fatal("engine does not support multiple prefixes")
	}
	input := []string{".", "*", ".", ".."}
	previous, err := controller.UpdatePrefix(input...)
	if err != nil || previous != "*" {
		t.Fatalf("previous=%q err=%v", previous, err)
	}
	input[0] = "broken"
	result := controller.GetPrefixes()
	result[0] = "broken"
	replicaPrefixes := any(replica).(interface{ GetPrefixes() []string }).GetPrefixes()
	if !reflect.DeepEqual(controller.GetPrefixes(), []string{".", "*", ".."}) || !reflect.DeepEqual(replicaPrefixes, []string{".", "*", ".."}) || host.GetPrefix() != "." {
		t.Fatalf("bad prefix snapshot: host=%v replica=%v", controller.GetPrefixes(), replicaPrefixes)
	}
	for _, invalid := range [][]string{nil, {""}, {"a b"}, {".\n"}, {"\x00"}} {
		if _, err := controller.UpdatePrefix(invalid...); err == nil {
			t.Fatalf("accepted invalid list %q", invalid)
		}
	}
	if !reflect.DeepEqual(controller.GetPrefixes(), []string{".", "*", ".."}) {
		t.Fatal("invalid update mutated state")
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				controller.UpdatePrefix(".", "*")
				controller.GetPrefixes()
				host.GetPrefix()
			}
		}()
	}
	wg.Wait()
	if _, err := controller.UpdatePrefix("$"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(controller.GetPrefixes(), []string{"$"}) {
		t.Fatal("single-prefix update retained aliases")
	}
}
