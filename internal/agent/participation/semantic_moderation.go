package participation

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"zenbot/internal/model"
)

// semanticModerationSignal is Saturn's fixed severe-abuse candidate gate without
// its Java Unicode word-boundary assertions. Go's \b is ASCII-only, so the
// source-compatible boundary checks are applied below.
var semanticModerationSignal = regexp.MustCompile(`(?i)(?:kys|kill\s+(?:yourself|urself|u|you)|hang\s+(?:yourself|urself)|doxx?|swat(?:ting)?|rape|shoot\s+you|stab\s+you|bomb\s+(?:you|them|the room))`)

func SemanticModerationCandidate(message model.ChatMessage) bool {
	for _, match := range semanticModerationSignal.FindAllStringIndex(message.Text, -1) {
		if !unicodeWordBoundaryBefore(message.Text, match[0]) {
			continue
		}
		candidate := strings.ToLower(message.Text[match[0]:match[1]])
		if requiresSourceWordBoundaryAfter(candidate) && !unicodeWordBoundaryAfter(message.Text, match[1]) {
			continue
		}
		return true
	}
	return false
}

func requiresSourceWordBoundaryAfter(candidate string) bool {
	switch candidate {
	case "dox", "doxx", "swat", "swatting", "rape":
		return true
	default:
		return false
	}
}

func unicodeWordBoundaryBefore(text string, index int) bool {
	if index == 0 {
		return true
	}
	previous, _ := utf8.DecodeLastRuneInString(text[:index])
	return !sourceWordRune(previous)
}

func unicodeWordBoundaryAfter(text string, index int) bool {
	if index == len(text) {
		return true
	}
	next, _ := utf8.DecodeRuneInString(text[index:])
	return !sourceWordRune(next)
}

// sourceWordRune mirrors Java's UNICODE_CHARACTER_CLASS \w ingredients used
// by Saturn's (?iu) pattern, including join controls.
func sourceWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || unicode.Is(unicode.Pc, r) || r == '\u200c' || r == '\u200d'
}

// SourceModerationAliases is the complete source run_command moderation
// inventory. It is deliberately separate from the public informational aliases.
func SourceModerationAliases() []string {
	return []string{"captcha", "mute", "unmute", "kick", "shadowban", "unshadowban"}
}

// SemanticModerationIngressReady remains false until each SourceModerationAlias
// has a reviewed typed Zenbot operation. Current EngineImpl operations cover
// captcha, mute, kick, and shadowban only; unmute and unshadowban have no
// source-backed typed operation and must not be invented.
func SemanticModerationIngressReady() bool { return false }
