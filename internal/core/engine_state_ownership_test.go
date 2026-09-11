package core

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"zenbot/internal/model"
	"zenbot/internal/transport"
)

func TestActiveStateInitializesOnFirstAdd(t *testing.T) {
	engine := &EngineImpl{}
	engine.AddActiveUser(&model.User{Name: "alice", Trip: "trip"})
	if engine.GetActiveUserByName("alice") == nil {
		t.Fatal("first active user was not added to zero-value engine")
	}
}

func TestActiveStateOwnsAddedUser(t *testing.T) {
	engine := &EngineImpl{ActiveUsers: map[*model.User]struct{}{}}
	joined := &model.User{Name: "alice", Trip: "trip", Hash: "hash"}
	engine.AddActiveUser(joined)
	joined.Name = "changed-input"
	if engine.GetActiveUserByName("alice") == nil {
		t.Fatal("mutating AddActiveUser input changed engine state")
	}
}

func TestActiveStateReturnsDetachedLookupUser(t *testing.T) {
	engine := &EngineImpl{}
	engine.ReplaceActiveUsers([]*model.User{{Name: "alice", Trip: "trip", Hash: "hash"}})
	byName := engine.GetActiveUserByName("alice")
	if byName == nil {
		t.Fatal("active user lookup failed")
	}
	byName.Name = "changed-lookup"
	if engine.GetActiveUserByName("alice") == nil {
		t.Fatal("mutating GetActiveUserByName result changed engine state")
	}
}

func TestActiveStateReturnsDetachedMapAndUsers(t *testing.T) {
	engine := &EngineImpl{}
	engine.ReplaceActiveUsers([]*model.User{{Name: "alice", Trip: "trip", Hash: "hash"}})
	snapshot := engine.GetActiveUsers()
	if snapshot == nil || len(*snapshot) != 1 {
		t.Fatalf("active snapshot = %#v", snapshot)
	}
	for user := range *snapshot {
		user.Name = "changed-snapshot"
		delete(*snapshot, user)
	}
	(*snapshot)[&model.User{Name: "mallory"}] = struct{}{}
	if engine.GetActiveUserByName("alice") == nil || engine.GetActiveUserByName("mallory") != nil {
		t.Fatal("mutating active snapshot changed engine state")
	}
}

func TestActiveStateCopiesReplacementRecords(t *testing.T) {
	engine := &EngineImpl{}
	alice := &model.User{Name: "alice", Trip: "trip", Hash: "hash"}
	engine.ReplaceActiveUsers([]*model.User{alice, nil})
	alice.Name = "changed-input"

	if engine.GetActiveUserByName("alice") == nil {
		t.Fatal("mutating ReplaceActiveUsers input changed engine state")
	}
}

func TestAfkStateInitializesOnFirstAdd(t *testing.T) {
	engine := &EngineImpl{}
	engine.AddAfkUser(&model.User{Name: "alice", Trip: "trip"}, "lunch")
	if got := len(*engine.GetAfkUsers()); got != 1 {
		t.Fatalf("AFK entries = %d, want 1", got)
	}
}

func TestAfkStateOwnsAddedUser(t *testing.T) {
	engine := &EngineImpl{AfkUsers: map[*model.User]string{}}
	alice := &model.User{Name: "alice", Trip: "trip", Hash: "hash"}
	engine.AddAfkUser(alice, "lunch")
	alice.Name = "changed-input"
	snapshot := engine.GetAfkUsers()
	if snapshot == nil || len(*snapshot) != 1 {
		t.Fatalf("AFK snapshot = %#v", snapshot)
	}
	for user, reason := range *snapshot {
		if user.Name != "alice" || reason != "lunch" {
			t.Fatalf("AFK entry = %#v, %q", user, reason)
		}
	}
}

func TestAfkStateReturnsDetachedMapAndUsers(t *testing.T) {
	engine := &EngineImpl{AfkUsers: map[*model.User]string{
		{Name: "alice", Trip: "trip", Hash: "hash"}: "lunch",
	}}
	snapshot := engine.GetAfkUsers()
	for user := range *snapshot {
		user.Name = "changed-snapshot"
		delete(*snapshot, user)
	}
	(*snapshot)[&model.User{Name: "mallory"}] = "spoofed"

	again := engine.GetAfkUsers()
	if len(*again) != 1 {
		t.Fatalf("mutating AFK snapshot changed size to %d", len(*again))
	}
	for user, reason := range *again {
		if user.Name != "alice" || reason != "lunch" {
			t.Fatalf("mutating AFK snapshot changed entry to %#v, %q", user, reason)
		}
	}
}

func TestAfkStateReaddingIdentityUpdatesReason(t *testing.T) {
	engine := &EngineImpl{AfkUsers: map[*model.User]string{}}
	engine.AddAfkUser(&model.User{Name: "alice", Trip: "trip", Hash: "hash"}, "lunch")
	engine.AddAfkUser(&model.User{Name: "alice", Trip: "trip", Hash: "new-hash"}, "meeting")
	updated := engine.GetAfkUsers()
	if len(*updated) != 1 {
		t.Fatalf("re-adding the same AFK identity created %d entries", len(*updated))
	}
	for user, reason := range *updated {
		if user.Name != "alice" || reason != "meeting" {
			t.Fatalf("updated AFK entry = %#v, %q", user, reason)
		}
	}
}

func TestAfkMentionIgnoresEmptyTrip(t *testing.T) {
	output := &stateOutputTransport{}
	engine := &EngineImpl{
		AfkUsers: map[*model.User]string{
			{Name: "alice"}: "lunch",
		},
		Transport: output,
	}

	engine.NotifyAfkIfMentioned(&model.ChatMessage{Name: "bob", Text: "unrelated message"})
	if got := output.count(); got != 0 {
		t.Fatalf("empty trip matched unrelated message and emitted %d notifications", got)
	}

	engine.NotifyAfkIfMentioned(&model.ChatMessage{Name: "bob", Text: "where is alice?"})
	if got := output.count(); got != 1 {
		t.Fatalf("name mention emitted %d notifications, want 1", got)
	}
}

func TestAfkRemovalReleasesStateLockBeforeOutput(t *testing.T) {
	output := newBlockingStateOutputTransport()
	t.Cleanup(output.unblock)
	engine := &EngineImpl{Transport: output}
	engine.AddAfkUser(&model.User{Name: "alice", Trip: "alice-trip"}, "lunch")

	removed := make(chan struct{})
	go func() {
		engine.RemoveIfAfk(&model.User{Name: "alice", Trip: "alice-trip"})
		close(removed)
	}()
	waitForSignal(t, output.started, "AFK removal output")

	added := make(chan struct{})
	go func() {
		engine.AddAfkUser(&model.User{Name: "bob", Trip: "bob-trip"}, "meeting")
		close(added)
	}()
	waitForSignal(t, added, "AFK writer while removal output was blocked")
	output.unblock()
	waitForSignal(t, removed, "AFK removal completion")

	snapshot := engine.GetAfkUsers()
	if len(*snapshot) != 1 {
		t.Fatalf("AFK entries after removal = %d, want 1", len(*snapshot))
	}
	for user := range *snapshot {
		if user.Name != "bob" {
			t.Fatalf("remaining AFK user = %q, want bob", user.Name)
		}
	}
}

func TestAfkMentionReleasesStateLockBeforeOutput(t *testing.T) {
	output := newBlockingStateOutputTransport()
	t.Cleanup(output.unblock)
	engine := &EngineImpl{Transport: output}
	engine.AddAfkUser(&model.User{Name: "alice", Trip: "alice-trip"}, "lunch")

	notified := make(chan struct{})
	go func() {
		engine.NotifyAfkIfMentioned(&model.ChatMessage{Name: "bob", Text: "alice"})
		close(notified)
	}()
	waitForSignal(t, output.started, "AFK mention output")

	added := make(chan struct{})
	go func() {
		engine.AddAfkUser(&model.User{Name: "carol", Trip: "carol-trip"}, "meeting")
		close(added)
	}()
	waitForSignal(t, added, "AFK writer while mention output was blocked")
	output.unblock()
	waitForSignal(t, notified, "AFK mention completion")
}

func TestStateConcurrentReadersAndWriters(t *testing.T) {
	engine := &EngineImpl{
		ActiveUsers: map[*model.User]struct{}{},
		AfkUsers:    map[*model.User]string{},
		Transport:   &stateOutputTransport{},
	}
	engine.AddActiveUser(&model.User{Name: "initial", Trip: "initial-trip"})
	engine.AddAfkUser(&model.User{Name: "initial", Trip: "initial-trip"}, "initial")

	var workers sync.WaitGroup
	workers.Add(4)
	go func() {
		defer workers.Done()
		for i := 0; i < 200; i++ {
			user := &model.User{Name: fmt.Sprintf("active-%d", i), Trip: fmt.Sprintf("trip-%d", i)}
			engine.AddActiveUser(user)
			engine.RemoveActiveUser(user)
			engine.ReplaceActiveUsers([]*model.User{{Name: "initial", Trip: "initial-trip"}})
		}
	}()
	go func() {
		defer workers.Done()
		for i := 0; i < 200; i++ {
			if user := engine.GetActiveUserByName("initial"); user != nil {
				user.Name = "detached"
			}
			for user := range *engine.GetActiveUsers() {
				user.Name = "detached"
			}
			_ = engine.ActiveUserNames()
		}
	}()
	go func() {
		defer workers.Done()
		for i := 0; i < 200; i++ {
			name := fmt.Sprintf("afk-%d", i)
			engine.AddAfkUser(&model.User{Name: name, Trip: fmt.Sprintf("afk-trip-%d", i)}, "away")
			engine.RenameAfkUser(name, name+"-renamed")
		}
	}()
	go func() {
		defer workers.Done()
		for i := 0; i < 200; i++ {
			for user := range *engine.GetAfkUsers() {
				user.Name = "detached"
			}
			engine.NotifyAfkIfMentioned(&model.ChatMessage{Name: "observer", Text: "no matching identity"})
		}
	}()
	workers.Wait()
}

type stateOutputTransport struct {
	mu          sync.Mutex
	payloads    []string
	started     chan struct{}
	release     chan struct{}
	startOnce   sync.Once
	releaseOnce sync.Once
}

func newBlockingStateOutputTransport() *stateOutputTransport {
	return &stateOutputTransport{started: make(chan struct{}), release: make(chan struct{})}
}

func (*stateOutputTransport) Start(context.Context) error               { return nil }
func (*stateOutputTransport) Messages() <-chan transport.InboundMessage { return nil }
func (*stateOutputTransport) Errors() <-chan error                      { return nil }
func (*stateOutputTransport) Connected() bool                           { return true }
func (t *stateOutputTransport) SendText(ctx context.Context, payload string) error {
	if t.started != nil {
		t.startOnce.Do(func() { close(t.started) })
	}
	if t.release != nil {
		select {
		case <-t.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	t.mu.Lock()
	t.payloads = append(t.payloads, payload)
	t.mu.Unlock()
	return nil
}
func (*stateOutputTransport) SendRaw(context.Context, []byte) error { return nil }
func (*stateOutputTransport) Close(context.Context) error           { return nil }

func (t *stateOutputTransport) count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.payloads)
}

func (t *stateOutputTransport) unblock() {
	if t.release != nil {
		t.releaseOnce.Do(func() { close(t.release) })
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", operation)
	}
}
