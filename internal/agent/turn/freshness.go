package turn

import (
	"regexp"
	"strings"
	"unicode"
	"zenbot/internal/agent/llm"
)

const UserMessageHistory = "user_message_history"

func IsInternalToolEvidence(s string) bool {
	return strings.HasPrefix(s, "[Internal tool evidence from ")
}
func LatestContent(ms []llm.LlmMessage, role string) string {
	for i := len(ms) - 1; i >= 0; i-- {
		if ms[i].Role() == role {
			return ms[i].Content()
		}
	}
	return ""
}
func LatestConversationAssistant(ms []llm.LlmMessage) string {
	for i := len(ms) - 1; i >= 0; i-- {
		if ms[i].Role() == "assistant" && !IsInternalToolEvidence(ms[i].Content()) {
			return ms[i].Content()
		}
	}
	return ""
}
func InternalToolEvidenceName(s string) string {
	const p = "[Internal tool evidence from "
	if !strings.HasPrefix(s, p) {
		return ""
	}
	x := strings.Index(s[len(p):], "]\n")
	if x <= 0 {
		return ""
	}
	return s[len(p) : len(p)+x]
}
func NormalizeNick(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\\_", "_"))
	return strings.TrimPrefix(s, "@")
}

var (
	profileRe      = regexp.MustCompile(`(?is)(?:tell\s+me\s+about|describe|profile|summari[sz]e|analy[sz]e)\s+(?:user\s+named\s+["']?)?@?([\p{L}\p{N}_-]{1,100})(?:[^\p{L}\p{N}_-]|$)`)
	namedUserRe    = regexp.MustCompile(`(?is)\buser\s+named\s+["']?@?([\p{L}\p{N}_-]{1,100})(?:\s+(?:profile|messages?|history|activity)\b|[?.!\s]*$)`)
	trailingUserRe = regexp.MustCompile(`(?is)(?:tell\s+me\s+about|describe|profile|summari[sz]e|analy[sz]e)\s+@?([\p{L}\p{N}_-]{1,100})\s+(?:user|member)\b`)
	whoRe          = regexp.MustCompile(`(?is)\bwho\s+is\s+@?([\p{L}\p{N}_-]{1,100})[?.!\s]*$`)
	speechRe       = regexp.MustCompile(`(?is)\bwhat\s+(?:did|has)\s+@?([\p{L}\p{N}_-]{1,100})\s+(?:say|said|post|posted|write|wrote|written)\b`)
	historyRe      = regexp.MustCompile(`(?is)\b(?:messages?|history|activity)\s+(?:of|for|from|by)\s+@?([\p{L}\p{N}_-]{1,100})(?:[^\p{L}\p{N}_-]|$)`)
	possessiveRe   = regexp.MustCompile(`(?is)(?:show(?:\s+me)?|give\s+me|describe|summari[sz]e|analy[sz]e)\s+@?([\p{L}\p{N}_-]{1,100})(?:'|’|')s\s+(?:profile|messages?|history|activity)\b`)
	followUpRe     = regexp.MustCompile(`(?is)^\s*(?:please\s+)?(?:(?:check|look\s+up)\s+(?:it|him|her|them|that)(?:\s+(?:again|elsewhere))?|do\s+it)(?:\s+@?[\p{L}\p{N}_-]{1,100})?[?.!\s]*$`)
)

var nonNickTerms = map[string]struct{}{"experience": {}, "interface": {}, "research": {}, "behavior": {}, "behaviour": {}, "java": {}, "here": {}, "there": {}, "shakespeare": {}, "rome": {}, "president": {}, "room": {}}

func extractedNick(prompt string) string {
	prompt = strings.ReplaceAll(prompt, "\\_", "_")
	for _, re := range []*regexp.Regexp{profileRe, namedUserRe, trailingUserRe, possessiveRe, whoRe, speechRe, historyRe} {
		if m := re.FindStringSubmatch(prompt); len(m) > 1 {
			n := NormalizeNick(m[1])
			if _, excluded := nonNickTerms[strings.ToLower(n)]; !excluded && IsValidNick(n) {
				return n
			}
		}
	}
	return ""
}

type FreshnessPolicy struct{}

func (FreshnessPolicy) Required(prompt string, history []llm.LlmMessage, users []string) (tool, nick string, ok bool) {
	if n := extractedNick(prompt); n != "" {
		return UserMessageHistory, n, true
	}
	lower := strings.ToLower(strings.TrimSpace(prompt))
	if followUpRe.MatchString(lower) {
		prev := LatestContent(history, "user")
		if n := extractedNick(prev); n != "" {
			return UserMessageHistory, n, true
		}
	}
	return "", "", false
}
func MatchesFreshCall(call llm.LlmToolCall, tool, nick string) bool {
	if call.Name() != tool {
		return false
	}
	if tool != UserMessageHistory {
		return true
	}
	a := call.Arguments()
	v, ok := a["nick"].(string)
	return ok && strings.EqualFold(NormalizeNick(v), NormalizeNick(nick))
}

// IsValidNick is the canonical freshness target bound used before any trusted lookup.
func IsValidNick(s string) bool {
	if len([]rune(s)) > 100 {
		return false
	}
	for _, r := range s {
		if !(unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-') {
			return false
		}
	}
	return s != ""
}
