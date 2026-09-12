package common

import (
	"fmt"
	"strings"
	"unicode"
)

// PrefixController replaces the validated live command prefix list on the host and its
// current managed concrete replicas.
type PrefixController interface {
	UpdatePrefix(...string) (previous string, err error)
}

// CommandPrefixes returns a detached snapshot, with a single-prefix fallback
// for legacy engine adapters. The first entry is the display/default prefix.
func CommandPrefixes(engine interface{ GetPrefix() string }) []string {
	if source, ok := engine.(interface{ GetPrefixes() []string }); ok {
		return append([]string(nil), source.GetPrefixes()...)
	}
	return []string{engine.GetPrefix()}
}

// MatchCommandPrefix ignores empty entries and resolves overlaps longest-first.
func MatchCommandPrefix(text string, prefixes []string) string {
	matched := ""
	for _, prefix := range prefixes {
		if len(prefix) > len(matched) && strings.HasPrefix(text, prefix) {
			matched = prefix
		}
	}
	return matched
}

// NormalizePrefixes validates tokens and deduplicates without changing priority.
func NormalizePrefixes(prefixes []string) ([]string, error) {
	if len(prefixes) == 0 {
		return nil, fmt.Errorf("at least one command prefix is required")
	}
	result := make([]string, 0, len(prefixes))
	seen := make(map[string]bool)
	for _, prefix := range prefixes {
		if prefix == "" || strings.ContainsFunc(prefix, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return nil, fmt.Errorf("command prefixes must be nonempty tokens without whitespace or control characters")
		}
		if !seen[prefix] {
			result = append(result, prefix)
			seen[prefix] = true
		}
	}
	return result, nil
}
