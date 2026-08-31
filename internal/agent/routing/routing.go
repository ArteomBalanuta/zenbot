// Package routing exposes the deterministic request policies without wiring a
// router to unsupported tool, turn, memory, or provider dependencies.
package routing

import (
	"strings"
	"zenbot/internal/agent/participation"
)

type RequestKind = participation.RequestKind
type ToolEvidence = participation.ToolEvidence
type Classifier = participation.Classifier

const (
	Unclassified = participation.Unclassified
	Talk         = participation.Talk
	ToolCall     = participation.ToolCall
)

// CommandIntentPolicy hides reflected Saturn command definitions unless the
// newest prompt explicitly names the command. Ordinary tools are retained.
type CommandIntentPolicy struct{}

func (CommandIntentPolicy) Filter(definitions []any, moderation bool, prompt string) []any {
	if moderation {
		return clone(definitions)
	}
	out := []any{}
	words := strings.Fields(prompt)
	for _, d := range definitions {
		name := definitionName(d)
		if !strings.HasPrefix(name, "saturn_") {
			out = append(out, d)
			continue
		}
		alias := strings.TrimPrefix(name, "saturn_")
		ok := len(words) > 0 && words[0] == alias || len(words) > 1 && (words[0] == "run" || words[0] == "execute") && words[1] == alias
		if ok {
			out = append(out, d)
		}
	}
	return clone(out)
}
func definitionName(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if f, ok := m["function"].(map[string]any); ok {
		if n, ok := f["name"].(string); ok {
			return n
		}
	}
	if n, ok := m["name"].(string); ok {
		return n
	}
	return ""
}
func clone(in []any) []any { out := make([]any, len(in)); copy(out, in); return out }
