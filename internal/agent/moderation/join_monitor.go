package moderation

import (
	"strings"
	"sync"
	"time"
	"unicode"

	"zenbot/internal/model"
)

type JoinConfig struct {
	Enabled                  bool
	JoinBurstCount           int
	JoinBurstWindow          time.Duration
	SameHashJoinCount        int
	SameHashJoinWindow       time.Duration
	SuspiciousNameJoinCount  int
	SuspiciousNameJoinWindow time.Duration
	PostKickWindow           time.Duration
	ActionCooldown           time.Duration
}

type JoinMonitor interface{ OnJoin(*model.User) []Decision }
type timedJoin struct {
	at   time.Time
	nick string
}
type actionKey struct {
	action    Action
	principal string
}

type monitor struct {
	config    JoinConfig
	now       func() time.Time
	protected func(*model.User) bool
	mu        sync.Mutex
	room      []timedJoin
	hashes    map[string][]timedJoin
	names     map[string][]timedJoin
	signals   map[string]time.Time
	actions   map[actionKey]time.Time
}

func NewJoinMonitor(config JoinConfig, now func() time.Time, protected func(*model.User) bool) JoinMonitor {
	if now == nil {
		now = time.Now
	}
	if protected == nil {
		protected = func(*model.User) bool { return false }
	}
	return &monitor{config: config, now: now, protected: protected, hashes: map[string][]timedJoin{}, names: map[string][]timedJoin{}, signals: map[string]time.Time{}, actions: map[actionKey]time.Time{}}
}
func (m *monitor) OnJoin(user *model.User) []Decision {
	if user == nil || !m.config.Enabled || m.protected(user) {
		return []Decision{}
	}
	now := m.now()
	j := timedJoin{now, user.Name}
	out := make([]Decision, 0, 2)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneExpired(now)
	m.room = append(m.room, j)
	if atLeast(m.room, m.config.JoinBurstCount) {
		m.addCaptcha(&out, "join burst", now)
		m.room = nil
	}
	if hash := strings.ToLower(strings.TrimSpace(user.Hash)); hash != "" {
		v := prune(m.hashes[hash], now, m.config.SameHashJoinWindow)
		v = append(v, j)
		if distinct(v) >= m.config.SameHashJoinCount {
			m.addCaptcha(&out, "same-hash nick variants", now)
			previous, had := m.signals[hash]
			m.signals[hash] = now
			if had && within(previous, now, m.config.PostKickWindow) && m.allow(ShadowBan, user.Name, now) {
				out = append(out, Decision{Action: ShadowBan, Principal: strings.TrimSpace(user.Name), Reason: "repeated same-hash raid"})
			}
			v = nil
		}
		m.hashes[hash] = v
	}
	if cluster := normalizeCluster(user.Name); len(cluster) >= 3 {
		v := prune(m.names[cluster], now, m.config.SuspiciousNameJoinWindow)
		v = append(v, j)
		if distinct(v) >= m.config.SuspiciousNameJoinCount {
			m.addCaptcha(&out, "suspicious name cluster", now)
			v = nil
		}
		m.names[cluster] = v
	}
	return out
}

// pruneExpired bounds every monitor-owned map, not only the bucket touched by
// the current join. Distinct one-off identities must age out without retaining
// unbounded per-user state.
func (m *monitor) pruneExpired(now time.Time) {
	m.room = prune(m.room, now, m.config.JoinBurstWindow)
	for hash, joins := range m.hashes {
		joins = prune(joins, now, m.config.SameHashJoinWindow)
		if len(joins) == 0 {
			delete(m.hashes, hash)
		} else {
			m.hashes[hash] = joins
		}
	}
	for cluster, joins := range m.names {
		joins = prune(joins, now, m.config.SuspiciousNameJoinWindow)
		if len(joins) == 0 {
			delete(m.names, cluster)
		} else {
			m.names[cluster] = joins
		}
	}
	for hash, at := range m.signals {
		if !within(at, now, m.config.PostKickWindow) {
			delete(m.signals, hash)
		}
	}
	for action, at := range m.actions {
		if !within(at, now, m.config.ActionCooldown) {
			delete(m.actions, action)
		}
	}
}
func (m *monitor) addCaptcha(out *[]Decision, reason string, now time.Time) {
	if m.allow(Captcha, "", now) {
		*out = append(*out, Decision{Action: Captcha, Reason: reason})
	}
}
func (m *monitor) allow(action Action, principal string, now time.Time) bool {
	k := actionKey{action, principal}
	previous, ok := m.actions[k]
	if ok && within(previous, now, m.config.ActionCooldown) {
		return false
	}
	m.actions[k] = now
	return true
}
func prune(v []timedJoin, now time.Time, window time.Duration) []timedJoin {
	cutoff := now.Add(-window)
	for len(v) > 0 && v[0].at.Before(cutoff) {
		v = v[1:]
	}
	return v
}
func atLeast(v []timedJoin, n int) bool { return n > 0 && len(v) >= n }
func distinct(v []timedJoin) int {
	s := map[string]struct{}{}
	for _, x := range v {
		s[strings.ToLower(strings.TrimSpace(x.nick))] = struct{}{}
	}
	return len(s)
}
func within(previous, now time.Time, window time.Duration) bool {
	d := now.Sub(previous)
	return d >= 0 && d <= window
}
func normalizeCluster(n string) string {
	n = strings.ToLower(strings.TrimSpace(n))
	var b strings.Builder
	for _, r := range n {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimRight(b.String(), "0123456789")
}
