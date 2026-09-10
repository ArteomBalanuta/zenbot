package moderation

import (
	"sync"
	"testing"
	"time"

	"zenbot/internal/model"
)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time          { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *testClock) Advance(d time.Duration) { c.mu.Lock(); c.now = c.now.Add(d); c.mu.Unlock() }

func join(name, trip, hash string) *model.User {
	return &model.User{Name: name, Trip: trip, Hash: hash}
}
func monitorConfig() JoinConfig {
	return JoinConfig{Enabled: true, JoinBurstCount: 3, JoinBurstWindow: time.Minute, SameHashJoinCount: 3, SameHashJoinWindow: time.Minute, SuspiciousNameJoinCount: 3, SuspiciousNameJoinWindow: time.Minute, PostKickWindow: time.Minute, ActionCooldown: time.Second}
}

func TestDecisionValidateActionTargetMatrix(t *testing.T) {
	for _, d := range []Decision{{Action: Captcha}, {Action: ShadowBan, Principal: "nick"}} {
		if err := d.Validate(); err != nil {
			t.Fatalf("%+v rejected: %v", d, err)
		}
	}
	for _, d := range []Decision{{Action: Captcha, Principal: "nick"}, {Action: ShadowBan}, {Action: ShadowBan, Principal: " nick "}, {Action: "unknown", Principal: "nick"}} {
		if err := d.Validate(); err == nil {
			t.Fatalf("%+v accepted", d)
		}
	}
}
func TestJoinMonitorDisabledAndProtectedDoNotAccumulate(t *testing.T) {
	clock := &testClock{now: time.Unix(0, 0)}
	cfg := monitorConfig()
	cfg.Enabled = false
	m := NewJoinMonitor(cfg, clock.Now, func(u *model.User) bool { return u.Trip == "admin" })
	for i := 0; i < 3; i++ {
		if got := m.OnJoin(join("p", "admin", "h")); len(got) != 0 {
			t.Fatal(got)
		}
	}
	cfg.Enabled = true
	m = NewJoinMonitor(cfg, clock.Now, func(u *model.User) bool { return u.Trip == "admin" })
	for i := 0; i < 3; i++ {
		m.OnJoin(join("p", "admin", "h"))
	}
	if got := m.OnJoin(join("a", "", "a")); len(got) != 0 {
		t.Fatal(got)
	}
}
func TestJoinMonitorBurstWindowResetCooldownAndEscalation(t *testing.T) {
	clock := &testClock{now: time.Unix(0, 0)}
	m := NewJoinMonitor(monitorConfig(), clock.Now, func(*model.User) bool { return false })
	for i := 0; i < 2; i++ {
		if got := m.OnJoin(join(string(rune('a'+i)), "", "h"+string(rune('a'+i)))); len(got) != 0 {
			t.Fatal(got)
		}
	}
	got := m.OnJoin(join("c", "", "hc"))
	if len(got) != 1 || got[0].Action != Captcha {
		t.Fatalf("%+v", got)
	}
	// A separate monitor avoids the preceding room-burst bucket; the second
	// same-hash wave is the source escalation condition.
	m = NewJoinMonitor(monitorConfig(), clock.Now, func(*model.User) bool { return false })
	for i := 0; i < 3; i++ {
		m.OnJoin(join("variant"+string(rune('a'+i)), "", "shared"))
	}
	clock.Advance(2 * time.Second)
	for i := 0; i < 3; i++ {
		got = m.OnJoin(join("other"+string(rune('a'+i)), "", "shared"))
	}
	if len(got) != 2 || got[0].Action != Captcha || got[1].Action != ShadowBan || got[1].Principal != "otherc" {
		t.Fatalf("%+v", got)
	}
}
func TestJoinMonitorConcurrentJoins(t *testing.T) {
	clock := &testClock{now: time.Unix(0, 0)}
	cfg := monitorConfig()
	cfg.JoinBurstCount = 10000
	m := NewJoinMonitor(cfg, clock.Now, func(*model.User) bool { return false })
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); m.OnJoin(join("n"+string(rune(i)), "", "h")) }(i)
	}
	wg.Wait()
}

func TestJoinMonitorPrunesExpiredStateAcrossDistinctPrincipals(t *testing.T) {
	clock := &testClock{now: time.Unix(0, 0)}
	cfg := monitorConfig()
	cfg.JoinBurstCount, cfg.SameHashJoinCount, cfg.SuspiciousNameJoinCount = 100, 100, 100
	m := NewJoinMonitor(cfg, clock.Now, func(*model.User) bool { return false }).(*monitor)
	for _, u := range []*model.User{
		join("alpha-one", "", "hash-one"),
		join("bravo-two", "", "hash-two"),
		join("charlie-three", "", "hash-three"),
	} {
		m.OnJoin(u)
	}
	if len(m.hashes) != 3 || len(m.names) != 3 {
		t.Fatalf("unexpected retained state hashes=%d names=%d", len(m.hashes), len(m.names))
	}
	clock.Advance(2 * time.Minute)
	m.OnJoin(join("current", "", "current-hash"))
	if len(m.room) != 1 || len(m.hashes) != 1 || len(m.names) != 1 {
		t.Fatalf("expired state was retained: room=%d hashes=%d names=%d", len(m.room), len(m.hashes), len(m.names))
	}
}

func TestNormalizeClusterKeepsOnlyUnicodeLettersAndNumbers(t *testing.T) {
	if got, want := normalizeCluster(" Raider😀123 "), "raider"; got != want {
		t.Fatalf("normalized cluster=%q, want %q", got, want)
	}
}
