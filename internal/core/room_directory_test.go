package core

import (
	"context"
	"sync"
	"testing"

	"zenbot/internal/model"
)

func roomDirectoryEngine(room string, names ...string) *EngineImpl {
	users := make(map[*model.User]struct{}, len(names))
	for _, name := range names {
		users[&model.User{Name: name}] = struct{}{}
	}
	return &EngineImpl{Channel: room, ActiveUsers: users}
}

func TestEngineRoomUserDirectoryFindsManagedHostAndReplicaSnapshots(t *testing.T) {
	host := roomDirectoryEngine("Lounge", "host")
	replica := roomDirectoryEngine("Games", "replica")
	manager := NewReplicaManager(host.Channel)
	if err := manager.Add(replica.Channel, managedReplica{replica}); err != nil {
		t.Fatal(err)
	}
	directory := EngineRoomUserDirectory{Host: host, Replicas: manager}

	for _, test := range []struct {
		lookup, room, user string
	}{
		{" lounge ", "Lounge", "host"},
		{"gAmEs", "Games", "replica"},
	} {
		snapshot, ok := directory.FindRoomUsers(test.lookup)
		if !ok || snapshot.Room != test.room || len(snapshot.Users) != 1 || snapshot.Users[0] != test.user {
			t.Fatalf("lookup %q: snapshot=%#v ok=%v", test.lookup, snapshot, ok)
		}
		snapshot.Users[0] = "mutated"
		copied, copiedOK := directory.FindRoomUsers(test.lookup)
		if !copiedOK || copied.Users[0] != test.user {
			t.Fatalf("lookup %q leaked mutable users: %q", test.lookup, copied.Users)
		}
	}
}

func TestEngineRoomUserDirectoryExcludesOpaqueAndRemovedReplicas(t *testing.T) {
	host := roomDirectoryEngine("host")
	manager := NewReplicaManager(host.Channel)
	if err := manager.Add("opaque", &testReplica{}); err != nil {
		t.Fatal(err)
	}
	replica := roomDirectoryEngine("replica", "alice")
	if err := manager.Add(replica.Channel, managedReplica{replica}); err != nil {
		t.Fatal(err)
	}
	directory := EngineRoomUserDirectory{Host: host, Replicas: manager}
	if _, ok := directory.FindRoomUsers("opaque"); ok {
		t.Fatal("opaque replica was exposed")
	}
	if _, ok := directory.FindRoomUsers(" "); ok {
		t.Fatal("blank lookup was accepted")
	}
	if _, ok := directory.FindRoomUsers("missing"); ok {
		t.Fatal("unknown lookup was accepted")
	}
	if _, err := manager.Remove(context.Background(), replica.Channel); err != nil {
		t.Fatal(err)
	}
	if _, ok := directory.FindRoomUsers(replica.Channel); ok {
		t.Fatal("removed replica remained visible")
	}
}

func TestEngineRoomUserDirectoryLookupRacesSafelyWithReplicaChanges(t *testing.T) {
	host := roomDirectoryEngine("host", "host-user")
	replica := roomDirectoryEngine("replica", "replica-user")
	manager := NewReplicaManager(host.Channel)
	directory := EngineRoomUserDirectory{Host: host, Replicas: manager}

	done := make(chan struct{})
	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
					_, _ = directory.FindRoomUsers("replica")
					_, _ = directory.FindRoomUsers("host")
				}
			}
		}()
	}
	for range 100 {
		if err := manager.Add(replica.Channel, managedReplica{replica}); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Remove(context.Background(), replica.Channel); err != nil {
			t.Fatal(err)
		}
	}
	close(done)
	readers.Wait()
}
