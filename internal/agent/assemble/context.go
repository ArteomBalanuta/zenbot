package assemble

import (
	"fmt"
	"sort"

	"zenbot/internal/agent/llm"
)

type ContextSource string

const (
	ContextMemory             ContextSource = "MEMORY"
	ContextHistoricalEvidence ContextSource = "HISTORICAL_EVIDENCE"
	ContextRecentRoom         ContextSource = "RECENT_ROOM"
	maxContextTokens                        = 1_000_000
)

// ContextUnit is the smallest removable context item. Messages in one unit
// are retained or removed together, preserving tool-call protocol pairs and
// complete structured payloads.
type ContextUnit struct {
	Source      ContextSource
	Timestamp   int64
	Fingerprint string
	Priority    int
	Messages    []Message
	order       int
}

type ContextInput struct {
	RequiredPrefix []Message
	Optional       []ContextUnit
	RequiredSuffix []Message
	MaxTokens      int
	ReserveTokens  int
	ManifestTokens int
}

type ContextBudgeter struct{}

func (ContextBudgeter) Project(input ContextInput) (Projection, error) {
	if input.MaxTokens <= 0 || input.MaxTokens > maxContextTokens {
		return Projection{}, fmt.Errorf("context max tokens must be between 1 and %d", maxContextTokens)
	}
	if input.ReserveTokens < 0 || input.ManifestTokens < 0 || input.ReserveTokens+input.ManifestTokens >= input.MaxTokens {
		return Projection{}, fmt.Errorf("context reserve and manifest tokens must be smaller than max tokens")
	}
	budgetChars := (input.MaxTokens - input.ReserveTokens - input.ManifestTokens) * 4
	prefix := copyMessages(input.RequiredPrefix)
	suffix := copyMessages(input.RequiredSuffix)
	requiredChars := serialized(prefix) + serialized(suffix)
	if requiredChars > budgetChars {
		return Projection{}, fmt.Errorf("required agent context exceeds token budget")
	}

	units := deduplicateUnits(input.Optional)
	ranked := append([]ContextUnit(nil), units...)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Priority != ranked[j].Priority {
			return ranked[i].Priority > ranked[j].Priority
		}
		if ranked[i].Timestamp != ranked[j].Timestamp {
			return ranked[i].Timestamp > ranked[j].Timestamp
		}
		return ranked[i].order > ranked[j].order
	})
	remaining := budgetChars - requiredChars
	selected := make(map[int]struct{}, len(ranked))
	for _, unit := range ranked {
		cost := serialized(unit.Messages)
		if cost <= remaining {
			selected[unit.order] = struct{}{}
			remaining -= cost
		}
	}
	sort.SliceStable(units, func(i, j int) bool { return units[i].order < units[j].order })
	messages := append([]Message(nil), prefix...)
	for _, unit := range units {
		if _, keep := selected[unit.order]; keep {
			messages = append(messages, copyMessages(unit.Messages)...)
		}
	}
	messages = append(messages, suffix...)
	chars := serialized(messages)
	removed := len(input.Optional) - len(selected)
	return Projection{
		Messages:        messages,
		SerializedChars: chars,
		EstimatedTokens: (chars + 3) / 4,
		BudgetChars:     budgetChars,
		RemovedUnits:    removed,
		Pruned:          removed > 0,
		Overflow:        false,
		Fingerprint:     fingerprint(messages),
	}, nil
}

func deduplicateUnits(source []ContextUnit) []ContextUnit {
	type candidate struct {
		unit ContextUnit
	}
	byFingerprint := make(map[string]candidate, len(source))
	for index, original := range source {
		unit := original
		unit.order = index
		unit.Messages = copyMessages(original.Messages)
		if unit.Fingerprint == "" {
			unit.Fingerprint = fingerprint(unit.Messages)
		}
		existing, found := byFingerprint[unit.Fingerprint]
		if !found || unit.Priority > existing.unit.Priority || (unit.Priority == existing.unit.Priority && unit.Timestamp >= existing.unit.Timestamp) {
			byFingerprint[unit.Fingerprint] = candidate{unit: unit}
		}
	}
	units := make([]ContextUnit, 0, len(byFingerprint))
	for _, item := range byFingerprint {
		units = append(units, item.unit)
	}
	return units
}

func copyMessages(source []Message) []Message {
	out := make([]Message, 0, len(source))
	for _, message := range source {
		out = append(out, llmMessageCopy(message))
	}
	return out
}

func llmMessageCopy(message Message) Message {
	return llm.NewLlmMessage(message.Role(), message.Content(), message.ToolCalls(), message.ToolCallID())
}
