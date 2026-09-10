package live

import (
	"regexp"
	"strings"
)

var (
	legacyOpening = regexp.MustCompile(`(?is)^\s*Ah,\s*[^\n.!?:]{1,80}[.!?:]\s*(?:You ask about\s+[^\n.!?]{1,120}[.!?]\s*)?`)
	legacySipTea  = regexp.MustCompile(`(?i)\[sips tea(?: slowly)?[^\]]*\]\s*`)
	initialMarkup = regexp.MustCompile("^\\s*(?:[*_~`#>]+\\s*)+")
	listItem      = regexp.MustCompile(`^[\t\p{Zs}]*(?:[*•]|[0-9]+[.)])[\t\p{Zs}]+(.+)$`)
	carpeDiem     = regexp.MustCompile(`(?i)^[*_]*carpe diem[*_]*[,.].*`)
)

type responseSanitizer struct{}

func (responseSanitizer) sanitize(raw string) string {
	if stripJavaWhitespace(raw) == "" {
		return ""
	}
	content := legacySipTea.ReplaceAllString(raw, "")
	content = initialMarkup.ReplaceAllString(content, "")
	content = stripJavaWhitespace(content)
	content = legacyOpening.ReplaceAllString(content, "")

	lines := strings.Split(content, "\n")
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		if isLegacyPersonaBoilerplate(line) {
			continue
		}
		if match := listItem.FindStringSubmatch(line); match != nil {
			clean = append(clean, "\u2009-\u2009"+match[1])
			continue
		}
		clean = append(clean, strings.TrimRightFunc(line, isJavaWhitespace))
	}
	return strings.TrimRightFunc(strings.Join(clean, "\n"), isJavaWhitespace)
}

func (responseSanitizer) containsLegacyPersona(raw string) bool {
	if stripJavaWhitespace(raw) == "" {
		return false
	}
	if strings.Contains(strings.ToLower(raw), "[sips tea") || legacyOpening.MatchString(stripJavaWhitespace(raw)) {
		return true
	}
	for _, line := range strings.Split(raw, "\n") {
		normalized := strings.ToLower(stripJavaWhitespace(line))
		if strings.HasPrefix(normalized, "the archives reveal") || carpeDiem.MatchString(normalized) {
			return true
		}
	}
	return false
}

func isLegacyPersonaBoilerplate(line string) bool {
	normalized := strings.ToLower(stripJavaWhitespace(line))
	return strings.HasPrefix(normalized, "the archives reveal") || normalized == "your history shows:" || carpeDiem.MatchString(normalized)
}

// isJavaWhitespace matches Character.isWhitespace, which String.strip and
// String.isBlank use in Saturn. In particular, it intentionally preserves
// non-breaking spaces that Go's unicode.IsSpace would remove.
func isJavaWhitespace(r rune) bool {
	switch {
	case r >= '	' && r <= '\r':
		return true
	case r >= '\x1c' && r <= '\x1f':
		return true
	case r == ' ' || r == '\u1680' || r == '\u2028' || r == '\u2029' || r == '\u205f' || r == '\u3000':
		return true
	case r >= '\u2000' && r <= '\u2006':
		return true
	case r >= '\u2008' && r <= '\u200a':
		return true
	default:
		return false
	}
}

func stripJavaWhitespace(s string) string {
	return strings.TrimFunc(s, isJavaWhitespace)
}
