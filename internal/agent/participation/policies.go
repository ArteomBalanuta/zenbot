// Package participation contains deterministic room participation policies and
// the narrow invocation/submission boundary. It is intentionally unwired from
// listeners until the remaining agent dependencies are available.
package participation

import (
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"zenbot/internal/agent/api"
)

type MentionParser struct{}

func (MentionParser) Parse(text, botNick string) (string, bool) {
	if strings.TrimSpace(text) == "" || strings.TrimSpace(botNick) == "" {
		return "", false
	}

	// Saturn recognizes every exact, case-insensitive @nick whose surrounding
	// characters are not letters, numbers, underscore, or (after the nick) '-'.
	runes := []rune(text)
	nick := []rune(strings.TrimSpace(botNick))
	var mentions [][2]int
	for i := 0; i+len(nick)+1 <= len(runes); i++ {
		if runes[i] != '@' || (i > 0 && mentionWordRune(runes[i-1])) {
			continue
		}
		end := i + 1 + len(nick)
		if !strings.EqualFold(string(runes[i+1:end]), string(nick)) {
			continue
		}
		if end < len(runes) && (mentionWordRune(runes[end]) || runes[end] == '-') {
			continue
		}
		mentions = append(mentions, [2]int{i, end})
		i = end - 1
	}
	if len(mentions) == 0 {
		return "", false
	}

	remaining := make([]rune, 0, len(runes))
	last := 0
	for _, mention := range mentions {
		remaining = append(remaining, runes[last:mention[0]]...)
		remaining = append(remaining, ' ')
		last = mention[1]
	}
	remaining = append(remaining, runes[last:]...)
	s := strings.TrimSpace(string(remaining))
	s = regexp.MustCompile(`^[\s,;:.-]+`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s+([?!.,])`).ReplaceAllString(s, "$1")
	if strings.HasSuffix(s, ",?") {
		s = strings.TrimSuffix(s, ",?") + "?"
	} else if strings.HasSuffix(s, ",!") {
		s = strings.TrimSuffix(s, ",!") + "!"
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	return s, true
}

func mentionWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_'
}

type QuietRegistry struct {
	mu       sync.Mutex
	duration time.Duration
	now      func() time.Time
	entries  map[string]time.Time
}

func NewQuietRegistry(duration time.Duration) *QuietRegistry {
	return NewQuietRegistryAt(duration, time.Now)
}
func NewQuietRegistryAt(duration time.Duration, now func() time.Time) *QuietRegistry {
	if duration <= 0 {
		panic("quiet duration must be positive")
	}
	return &QuietRegistry{duration: duration, now: now, entries: map[string]time.Time{}}
}
func quietText(s string) bool {
	s = strings.ToLower(strings.ReplaceAll(s, "’", "'"))
	polite := regexp.MustCompile(`\b(?:please|kindly|could you|would you|can you)\b`).MatchString(s)
	intent := regexp.MustCompile(`\b(?:be (?:quiet|silent)|stay (?:quiet|silent)|remain silent|keep quiet|stop talking|do not join|don't join|stay out|leave (?:me|us) alone|do not interrupt|don't interrupt)\b`).MatchString(s)
	return polite && intent
}
func (q *QuietRegistry) IsPoliteQuietRequest(text, _ string) bool {
	return strings.TrimSpace(text) != "" && quietText(text)
}
func quietKey(c api.Context) string {
	id, _ := api.FromContext(&c)
	return strings.ToLower(strings.TrimSpace(c.Room())) + "|" + id.Value()
}
func (q *QuietRegistry) Silence(c api.Context) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.entries[quietKey(c)] = q.now().Add(q.duration)
}
func (q *QuietRegistry) IsQuiet(c api.Context) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	k := quietKey(c)
	until, ok := q.entries[k]
	if !ok {
		return false
	}
	if until.After(q.now()) {
		return true
	}
	delete(q.entries, k)
	return false
}

type RequestKind string

const (
	Unclassified RequestKind = "UNCLASSIFIED"
	Talk         RequestKind = "TALK"
	ToolCall     RequestKind = "TOOL_CALL"
)

type ToolEvidence struct{ Attempted bool }
type Classifier struct{}

func (Classifier) Classify(text string) RequestKind {
	s := strings.TrimSpace(text)
	if s == "" || !hasLetter(s) || hasControl(s) || protocol(s) || action(s) {
		return Unclassified
	}
	if social(s) || strings.ContainsAny(s, ".!?。！？") {
		return Talk
	}
	return Unclassified
}
func (Classifier) Finalize(candidate RequestKind, e ToolEvidence) RequestKind {
	if e.Attempted {
		return ToolCall
	}
	if candidate == Talk {
		return Talk
	}
	return Unclassified
}
func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}
func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
func protocol(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "{") || strings.HasPrefix(l, "[") || strings.HasPrefix(l, "<") || strings.HasPrefix(l, "```") || strings.HasPrefix(l, "tool_call") || strings.HasPrefix(l, "function_call")
}
func action(s string) bool {
	return regexp.MustCompile(`(?i)^(run|execute|do|make|create|delete|remove|set|get|find|search|lookup|list|show|check|send|post|remember|schedule|weather|who is|what is the weather)\b`).MatchString(s)
}
func social(s string) bool {
	return regexp.MustCompile(`(?i)^(how are you|what do you think|can you explain|why|how)\b.*\?|.*\b(hello|hi|hey|thanks|thank you|goodbye|bye|okay|ok)\b.*`).MatchString(s)
}
