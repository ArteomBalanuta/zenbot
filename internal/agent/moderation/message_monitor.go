package moderation

import (
	"strings"
	"sync"
	"time"
	"unicode"

	"zenbot/internal/model"
)

// MessageConfig contains the bounded deterministic public-message detector settings.
type MessageConfig struct {
	Enabled            bool
	BurstCount         int
	BurstWindow        time.Duration
	RepeatedCount      int
	RepeatedWindow     time.Duration
	SecondBreachWindow time.Duration
	PostKickWindow     time.Duration
	ActionCooldown     time.Duration
}

type MessageMonitor interface {
	OnMessage(model.ChatMessage) []Decision
}

type timedMessage struct {
	at         time.Time
	normalized string
}

type offence struct {
	action Action
	at     time.Time
}

type messageMonitor struct {
	config    MessageConfig
	now       func() time.Time
	protected func(model.ChatMessage) bool
	mu        sync.Mutex
	messages  map[string][]timedMessage
	offences  map[string]offence
	actions   map[actionKey]time.Time
}

func NewMessageMonitor(config MessageConfig, now func() time.Time, protected func(model.ChatMessage) bool) *messageMonitor {
	if now == nil {
		now = time.Now
	}
	if protected == nil {
		protected = func(model.ChatMessage) bool { return false }
	}
	return &messageMonitor{config: config, now: now, protected: protected, messages: map[string][]timedMessage{}, offences: map[string]offence{}, actions: map[actionKey]time.Time{}}
}

func (m *messageMonitor) OnMessage(message model.ChatMessage) []Decision {
	if m == nil || !m.config.Enabled || message.Whisper || message.IsWhisper || m.protected(message) {
		return []Decision{}
	}
	principal, identity := messagePrincipal(message)
	normalized := normalizeMessage(message.Text)
	if principal == "" || identity == "" || normalized == "" {
		return []Decision{}
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneExpired(now)
	recent := append(m.messages[identity], timedMessage{at: now, normalized: normalized})
	recent = pruneMessages(recent, now, maxDuration(m.config.BurstWindow, m.config.RepeatedWindow))
	m.messages[identity] = recent
	burst, repeated := countMessages(recent, now, m.config.BurstWindow), countRepeated(recent, normalized, now, m.config.RepeatedWindow)
	if !atLeastMessages(burst, m.config.BurstCount) && !atLeastMessages(repeated, m.config.RepeatedCount) {
		return []Decision{}
	}
	m.messages[identity] = nil
	if d, ok := m.escalate(identity, principal, repeated >= m.config.RepeatedCount, now); ok {
		return []Decision{d}
	}
	return []Decision{}
}

func (m *messageMonitor) escalate(identity, principal string, repeated bool, now time.Time) (Decision, bool) {
	previous, hasPrevious := m.offences[identity]
	if hasPrevious && previous.action == ShadowBan {
		return Decision{}, false
	}
	action := Warn
	if hasPrevious && previous.action == Kick && within(previous.at, now, m.config.PostKickWindow) {
		action = ShadowBan
	} else if hasPrevious && previous.action == Mute && within(previous.at, now, m.config.SecondBreachWindow) {
		action = Kick
	} else if hasPrevious && previous.action == Warn && within(previous.at, now, m.config.SecondBreachWindow) {
		action = Mute
	} else if repeated {
		action = Mute
	}
	if !m.allow(action, identity, now) {
		return Decision{}, false
	}
	m.offences[identity] = offence{action: action, at: now}
	reason := "message burst"
	if repeated {
		reason = "repeated message spam"
	}
	return Decision{Action: action, Principal: principal, Reason: reason}, true
}

func (m *messageMonitor) allow(action Action, identity string, now time.Time) bool {
	key := actionKey{action: action, principal: identity}
	if previous, ok := m.actions[key]; ok && within(previous, now, m.config.ActionCooldown) {
		return false
	}
	m.actions[key] = now
	return true
}

func (m *messageMonitor) pruneExpired(now time.Time) {
	window := maxDuration(m.config.BurstWindow, m.config.RepeatedWindow)
	for identity, messages := range m.messages {
		if messages = pruneMessages(messages, now, window); len(messages) == 0 {
			delete(m.messages, identity)
		} else {
			m.messages[identity] = messages
		}
	}
	for identity, previous := range m.offences {
		limit := m.config.SecondBreachWindow
		if previous.action == Kick || previous.action == ShadowBan {
			limit = m.config.PostKickWindow
		}
		if !within(previous.at, now, limit) {
			delete(m.offences, identity)
		}
	}
	for key, previous := range m.actions {
		if !within(previous, now, m.config.ActionCooldown) {
			delete(m.actions, key)
		}
	}
}

func messagePrincipal(message model.ChatMessage) (string, string) {
	principal := strings.TrimSpace(message.Name)
	if principal == "" {
		return "", ""
	}
	if hash := strings.ToLower(strings.TrimSpace(message.Hash)); hash != "" {
		return principal, "hash:" + hash
	}
	if trip := strings.ToLower(strings.TrimSpace(message.Trip)); trip != "" {
		return principal, "trip:" + trip
	}
	return principal, "nick:" + strings.ToLower(principal)
}
func normalizeMessage(text string) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, `\n`, " "), "\n", " ")
	return strings.Join(strings.FieldsFunc(strings.ToLower(strings.TrimSpace(text)), unicode.IsSpace), " ")
}
func pruneMessages(values []timedMessage, now time.Time, window time.Duration) []timedMessage {
	cutoff := now.Add(-window)
	for len(values) > 0 && values[0].at.Before(cutoff) {
		values = values[1:]
	}
	return values
}
func countMessages(values []timedMessage, now time.Time, window time.Duration) int {
	cutoff := now.Add(-window)
	n := 0
	for _, value := range values {
		if !value.at.Before(cutoff) {
			n++
		}
	}
	return n
}
func countRepeated(values []timedMessage, normalized string, now time.Time, window time.Duration) int {
	cutoff := now.Add(-window)
	n := 0
	for _, value := range values {
		if !value.at.Before(cutoff) && value.normalized == normalized {
			n++
		}
	}
	return n
}
func atLeastMessages(n, threshold int) bool { return threshold > 0 && n >= threshold }
func maxDuration(a, b time.Duration) time.Duration {
	if a >= b {
		return a
	}
	return b
}
