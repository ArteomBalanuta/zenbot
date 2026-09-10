package assemble

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/turn"
)

type ContextSource string

const (
	ContextMemory             ContextSource = "MEMORY"
	ContextHistoricalEvidence ContextSource = "HISTORICAL_EVIDENCE"
	ContextRecentRoom         ContextSource = "RECENT_ROOM"
	ContextObservation        ContextSource = "OBSERVATION"
	ContextCorrection         ContextSource = "CORRECTION"
	maxContextTokens                        = 1_000_000
	requestEnvelopeChars                    = 64
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
	tail        bool
}

type ContextInput struct {
	RequiredPrefix []Message
	Optional       []ContextUnit
	RequiredSuffix []Message
	OptionalTail   []ContextUnit
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
	requiredChars := serialized(prefix) + serialized(suffix) + requestEnvelopeChars
	if requiredChars > budgetChars {
		return Projection{}, fmt.Errorf("required agent context exceeds token budget")
	}

	allOptional := append([]ContextUnit(nil), input.Optional...)
	for _, original := range input.OptionalTail {
		unit := original
		unit.tail = true
		allOptional = append(allOptional, unit)
	}
	units := deduplicateUnits(allOptional)
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
		if _, keep := selected[unit.order]; keep && !unit.tail {
			messages = append(messages, copyMessages(unit.Messages)...)
		}
	}
	messages = append(messages, suffix...)
	for _, unit := range units {
		if _, keep := selected[unit.order]; keep && unit.tail {
			messages = append(messages, copyMessages(unit.Messages)...)
		}
	}
	chars := serialized(messages) + requestEnvelopeChars
	removed := len(allOptional) - len(selected)
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

// ProjectTurn is the single transcript projection boundary used before model
// calls. Required policy, task state, and newest request are retained while
// assistant tool calls and their observations remain atomic optional units.
func ProjectTurn(messages []Message, tools []any, taskState *turn.TaskState, observations *ObservationStore, input ContextInput) (Projection, error) {
	input.ManifestTokens = manifestTokenCost(tools)
	if taskState != nil {
		input.RequiredPrefix = append(copyMessages(input.RequiredPrefix), taskStateMessage(taskState))
	}
	sanitized := projectedObservationMessages(messages, observations)
	before, after := splitTurnContext(sanitized, input.RequiredPrefix, input.RequiredSuffix)
	input.Optional = append(input.Optional, before...)
	input.OptionalTail = append(input.OptionalTail, after...)
	return (ContextBudgeter{}).Project(input)
}

func manifestTokenCost(tools []any) int {
	if len(tools) == 0 {
		return 0
	}
	encoded, err := json.Marshal(tools)
	if err != nil {
		return maxContextTokens
	}
	return (len(encoded) + 3) / 4
}

func taskStateMessage(state *turn.TaskState) Message {
	task := state.Contract()
	satisfiedEvidence := make([]string, 0)
	for _, evidence := range state.Evidence() {
		if evidence.ObligationID != "" {
			satisfiedEvidence = append(satisfiedEvidence, evidence.ObligationID+":"+evidence.CallID)
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"requestId":            task.RequestID,
		"requestHash":          task.RequestHash,
		"objective":            task.Objective,
		"constraints":          task.Constraints(),
		"pendingObligations":   state.Pending(),
		"satisfiedEvidence":    satisfiedEvidence,
		"unknownActionOutcome": state.HasUnknownActionOutcome(),
		"prohibitedActionRetries": func() []string {
			if state.HasUnknownActionOutcome() {
				return []string{"all action retries for this turn"}
			}
			return []string{}
		}(),
	})
	return llm.NewLlmMessage("system", "TASK_STATE_JSON="+string(payload), nil, "")
}

func projectedObservationMessages(messages []Message, observations *ObservationStore) []Message {
	out := copyMessages(messages)
	if observations == nil {
		return out
	}
	for index, message := range out {
		if message.Role() != "tool" || message.ToolCallID() == "" {
			continue
		}
		if view, found := observations.View(message.ToolCallID()); found {
			out[index] = llm.NewLlmMessage("tool", string(view.JSON()), nil, message.ToolCallID())
		}
	}
	return out
}

func splitTurnContext(messages, requiredPrefix, requiredSuffix []Message) ([]ContextUnit, []ContextUnit) {
	prefixIndices := matchRequiredFromStart(messages, requiredPrefix, nil)
	suffixIndices := matchRequiredFromEnd(messages, requiredSuffix)
	for index := range suffixIndices {
		delete(prefixIndices, index)
	}
	suffixEnd := -1
	for index := range suffixIndices {
		if index > suffixEnd {
			suffixEnd = index
		}
	}
	remaining := make([]Message, 0, len(messages))
	newestSeen := false
	positions := make([]bool, 0, len(messages))
	for index, message := range messages {
		if _, required := prefixIndices[index]; required {
			continue
		}
		if _, required := suffixIndices[index]; required {
			if index == suffixEnd {
				newestSeen = true
			}
			continue
		}
		if message.Role() == "system" && strings.HasPrefix(message.Content(), "TASK_STATE_JSON=") {
			continue
		}
		remaining = append(remaining, message)
		positions = append(positions, newestSeen)
	}
	before := make([]ContextUnit, 0)
	after := make([]ContextUnit, 0)
	for index := 0; index < len(remaining); {
		message := remaining[index]
		if message.Role() == "tool" {
			index++
			continue
		}
		unitMessages := []Message{message}
		end := index + 1
		if len(message.ToolCalls()) > 0 {
			wanted := make(map[string]bool, len(message.ToolCalls()))
			for _, call := range message.ToolCalls() {
				wanted[call.ID()] = true
			}
			seen := make(map[string]int, len(wanted))
			for end < len(remaining) && remaining[end].Role() == "tool" {
				if wanted[remaining[end].ToolCallID()] {
					seen[remaining[end].ToolCallID()]++
					unitMessages = append(unitMessages, remaining[end])
				}
				end++
			}
			valid := len(unitMessages) == len(wanted)+1
			for callID := range wanted {
				valid = valid && seen[callID] == 1
			}
			if !valid {
				index = end
				continue
			}
		}
		tail := positions[index]
		priority, source := 50, ContextMemory
		if len(message.ToolCalls()) > 0 {
			priority, source = 100, ContextObservation
		} else if tail {
			priority, source = 90, ContextCorrection
		}
		unit := ContextUnit{Source: source, Priority: priority, Messages: unitMessages}
		if tail {
			after = append(after, unit)
		} else {
			before = append(before, unit)
		}
		index = end
	}
	return before, after
}

func matchRequiredFromStart(messages, required []Message, excluded map[int]struct{}) map[int]struct{} {
	indices := make(map[int]struct{}, len(required))
	cursor := 0
	for _, wanted := range required {
		for cursor < len(messages) {
			index := cursor
			cursor++
			if _, skip := excluded[index]; skip {
				continue
			}
			if messageFingerprint(messages[index]) == messageFingerprint(wanted) {
				indices[index] = struct{}{}
				break
			}
		}
	}
	return indices
}

func matchRequiredFromEnd(messages, required []Message) map[int]struct{} {
	indices := make(map[int]struct{}, len(required))
	cursor := len(messages) - 1
	for requiredIndex := len(required) - 1; requiredIndex >= 0; requiredIndex-- {
		wanted := required[requiredIndex]
		for cursor >= 0 {
			index := cursor
			cursor--
			if messageFingerprint(messages[index]) == messageFingerprint(wanted) {
				indices[index] = struct{}{}
				break
			}
		}
	}
	return indices
}

func messageFingerprint(message Message) string {
	return fingerprint([]Message{message})
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
