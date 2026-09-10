// Package assemble is the pure, private boundary between trusted runtime state and an LLM request.
package assemble

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/participation"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/agent/turn"
)

// Catalog is the prompt resource seam. prompt.Catalog satisfies it directly.
type Catalog interface {
	Text(string) (string, error)
	Formatted(string, ...any) (string, error)
}

// Config contains only assembly policy; it deliberately has no provider or command dependencies.
type Config struct {
	CreatorTrip, NoReplyMarker string
	MaxPromptChars             int
	MaxContextTokens           int
	ContextReserveTokens       int
}

// RequestKind identifies the trusted classification metadata carried in the system prompt.
type RequestKind string

const (
	Unclassified RequestKind = "UNCLASSIFIED"
	Talk         RequestKind = "TALK"
	Command      RequestKind = "COMMAND"
)

// ToolEvidence is retained for compatibility with callers that still carry
// execution accounting. Request-local loop state is not rendered into prompts.
type ToolEvidence struct {
	Attempted                                    bool
	AttemptedCount, SuccessfulCount, FailedCount int
}

// SystemPrompt renders the ordered Saturn system prompt sections.
type SystemPrompt struct {
	config  Config
	catalog Catalog
}

func NewSystemPrompt(config Config, catalog Catalog) (*SystemPrompt, error) {
	if catalog == nil {
		return nil, errors.New("prompt catalog must not be nil")
	}
	if config.NoReplyMarker == "" {
		config.NoReplyMarker = "NO_REPLY"
	}
	return &SystemPrompt{config: config, catalog: catalog}, nil
}
func NewSystemPromptWithCatalog(config Config, catalog Catalog) *SystemPrompt {
	p, _ := NewSystemPrompt(config, catalog)
	return p
}
func (p *SystemPrompt) Render(inv runtime.Invocation, _ string, _ string, kind RequestKind, _ ToolEvidence, _ string) (string, error) {
	if p == nil || p.catalog == nil {
		return "", errors.New("prompt catalog must not be nil")
	}
	ctx := inv.Context()
	caller := map[string]any{
		"nick":              ctx.Nick(),
		"isCreator":         p.config.CreatorTrip != "" && p.config.CreatorTrip == ctx.Trip(),
		"capabilities":      ctx.Capabilities(),
		"canModerate":       ctx.HasCapability(runtime.ModerationCommands),
		"canPermanentlyBan": ctx.HasCapability(runtime.PermanentBan),
		"canAdminister":     ctx.HasCapability(runtime.AdminCommands),
	}
	runtimeMeta := map[string]any{"invocationMode": string(inv.Mode()), "requestKind": string(kind), "room": ctx.Room(), "whisper": ctx.Whisper(), "caller": caller}
	meta, err := json.Marshal(runtimeMeta)
	if err != nil {
		return "", err
	}
	db := "system/database-policy-disabled.txt"
	if ctx.HasCapability(runtime.DynamicSQL) {
		db = "system/database-policy-enabled.txt"
	}
	database, err := p.catalog.Text(db)
	if err != nil {
		return "", err
	}
	database = strings.TrimSpace(database)
	part := map[runtime.Mode]string{runtime.DIRECT: "system/participation-direct.txt", runtime.MENTION: "system/participation-mention.txt"}[inv.Mode()]
	var participation string
	if part != "" {
		participation, err = p.catalog.Text(part)
		if err != nil {
			return "", err
		}
		participation = strings.TrimSpace(participation)
	} else if inv.Mode() == runtime.AMBIENT {
		participation, err = p.catalog.Formatted("system/participation-ambient.txt", p.config.NoReplyMarker, p.config.NoReplyMarker)
	} else if inv.Mode() == runtime.MODERATION {
		participation, err = p.catalog.Formatted("system/participation-moderation.txt", p.config.NoReplyMarker, p.config.NoReplyMarker)
	} else {
		return "", fmt.Errorf("invalid invocation mode: %s", inv.Mode())
	}
	if err != nil {
		return "", err
	}
	persona, err := p.catalog.Text("persona/vaelen-system-prompt.txt")
	if err != nil {
		return "", err
	}
	persona = strings.TrimSpace(persona)
	policy, err := p.catalog.Formatted("system/system-policy.txt", map[bool]string{true: "private whisper", false: "shared room"}[ctx.Whisper()], database, participation, persona, string(meta))
	if err != nil {
		return "", err
	}
	return policy, nil
}

// RenderWithHistoricalEvidence is retained for compatibility. Untrusted data
// is projected by Assembler as user-role context, never into the system role.
func (p *SystemPrompt) RenderWithHistoricalEvidence(inv runtime.Invocation, correlationID, recent string, kind RequestKind, evidence ToolEvidence, phase string, historical []turn.HistoricalEvidence) (string, error) {
	return p.Render(inv, correlationID, "", kind, evidence, phase)
}

// Message is an immutable provider-neutral message used by assembly.
type Message = llm.LlmMessage

// Projection is immutable accounting for projected context.
type Projection struct {
	Messages                                                    []Message
	SerializedChars, EstimatedTokens, BudgetChars, RemovedUnits int
	Pruned, Overflow                                            bool
	Fingerprint                                                 string
}

func (p Projection) MessagesCopy() []Message { return append([]Message(nil), p.Messages...) }

func project(source []Message, budget int) Projection {
	if budget < 0 {
		budget = 0
	}
	if len(source) == 0 {
		return Projection{BudgetChars: budget, Fingerprint: fingerprint(nil)}
	}
	copyMsg := func(m Message) Message {
		return llm.NewLlmMessage(m.Role(), m.Content(), m.ToolCalls(), m.ToolCallID())
	}
	out := make([]Message, 0, len(source))
	units := [][]Message{}
	if source[0].Role() != "system" {
		for _, m := range source {
			out = append(out, copyMsg(m))
		}
	} else {
		out = append(out, copyMsg(source[0]))
		for i := 1; i < len(source)-1; i++ {
			m := source[i]
			if len(m.ToolCalls()) > 0 {
				unit := []Message{copyMsg(m)}
				ids := map[string]bool{}
				for _, c := range m.ToolCalls() {
					ids[c.ID()] = true
				}
				seen := map[string]int{}
				j := i + 1
				for j < len(source)-1 && source[j].Role() == "tool" {
					id := source[j].ToolCallID()
					seen[id]++
					if ids[id] {
						unit = append(unit, copyMsg(source[j]))
					}
					j++
				}
				valid := true
				for id := range ids {
					valid = valid && seen[id] == 1
				}
				if valid {
					units = append(units, unit)
				}
				i = j - 1
			} else if m.Role() != "tool" {
				units = append(units, []Message{copyMsg(m)})
			}
		}
		for _, u := range units {
			out = append(out, u...)
		}
		out = append(out, copyMsg(source[len(source)-1]))
	}
	removed := 0
	for serialized(out) > budget && len(out) > 2 && len(units) > 0 {
		n := len(units[0])
		out = append(out[:1], out[1+n:]...)
		units = units[1:]
		removed++
	}
	chars := serialized(out)
	return Projection{Messages: out, SerializedChars: chars, EstimatedTokens: (chars + 3) / 4, BudgetChars: budget, Pruned: removed > 0, Overflow: chars > budget, RemovedUnits: removed, Fingerprint: fingerprint(out)}
}
func serialized(ms []Message) int {
	encoded, err := json.Marshal(messageWireFormat(ms))
	if err != nil {
		return maxContextTokens * 4
	}
	return len(encoded)
}
func fingerprint(ms []Message) string {
	h := sha256.New()
	for _, m := range ms {
		h.Write([]byte(m.Role() + "|" + m.Content() + "|" + m.ToolCallID() + fmt.Sprint(m.ToolCalls())))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func messageWireFormat(messages []Message) []map[string]any {
	out := make([]map[string]any, len(messages))
	for index, message := range messages {
		item := map[string]any{"role": message.Role(), "content": message.ContentNullable()}
		if message.ToolCallID() != "" {
			item["tool_call_id"] = message.ToolCallID()
		}
		if calls := message.ToolCalls(); len(calls) > 0 {
			toolCalls := make([]map[string]any, len(calls))
			for callIndex, call := range calls {
				arguments, _ := json.Marshal(call.Arguments())
				toolCalls[callIndex] = map[string]any{
					"id": call.ID(), "type": "function",
					"function": map[string]any{"name": call.Name(), "arguments": string(arguments)},
				}
			}
			item["tool_calls"] = toolCalls
		}
		out[index] = item
	}
	return out
}

// PreparedRequest is immutable request state prepared before provider execution.
type PreparedRequest struct {
	messages       []Message
	tools          []any
	contextualized string
	kind           RequestKind
	projection     Projection
}

func (r PreparedRequest) Messages() []Message          { return append([]Message(nil), r.messages...) }
func (r PreparedRequest) Tools() []any                 { return cloneAnySlice(r.tools) }
func (r PreparedRequest) ContextualizedPrompt() string { return r.contextualized }
func (r PreparedRequest) RequestKind() RequestKind     { return r.kind }
func (r PreparedRequest) Projection() Projection       { return r.projection }
func (r PreparedRequest) LlmRequest() llm.LlmRequest {
	return llm.NewLlmRequest(r.messages, r.tools, false, nil, r.projection)
}

// Assembler builds requests without dispatch, repositories, network, or runtime orchestration.
type Assembler struct {
	config  Config
	catalog Catalog
	system  *SystemPrompt
}

func New(config Config, catalog Catalog) (*Assembler, error) {
	s, e := NewSystemPrompt(config, catalog)
	if e != nil {
		return nil, e
	}
	return &Assembler{config: config, catalog: catalog, system: s}, nil
}
func (a *Assembler) Assemble(ctx context.Context, inv runtime.Invocation, history []Message, recent string, tools []any, kind RequestKind) (PreparedRequest, error) {
	return a.AssembleWithHistoricalEvidence(ctx, inv, history, recent, tools, kind, nil)
}

// AssembleWithHistoricalEvidence adds only validated historical tool data to the initial request.
func (a *Assembler) AssembleWithHistoricalEvidence(ctx context.Context, inv runtime.Invocation, history []Message, recent string, tools []any, kind RequestKind, historical []turn.HistoricalEvidence) (PreparedRequest, error) {
	if err := ctx.Err(); err != nil {
		return PreparedRequest{}, err
	}
	if a == nil || a.system == nil {
		return PreparedRequest{}, errors.New("assembler is not initialized")
	}
	if a.config.MaxPromptChars > 0 && utf8.RuneCountInString(inv.Prompt()) > a.config.MaxPromptChars {
		return PreparedRequest{}, fmt.Errorf("agent prompt character limit exceeded")
	}
	maxTokens := a.maxContextTokens()
	sys, e := a.system.RenderWithHistoricalEvidence(inv, inv.RequestID(), "", kind, ToolEvidence{}, "CANDIDATE", nil)
	if e != nil {
		return PreparedRequest{}, e
	}
	if err := ctx.Err(); err != nil {
		return PreparedRequest{}, err
	}
	ctxr := inv.Context()
	promptText, e := a.catalog.Formatted("input/router-contextualized-prompt.txt", map[bool]string{true: "Private Saturn whisper", false: "Public Saturn message"}[ctxr.Whisper()], ctxr.Nick(), ctxr.Room(), inv.Prompt())
	if e != nil {
		return PreparedRequest{}, e
	}
	optional := historyContextUnits(history)
	if !ctxr.Whisper() {
		if historicalMessage, ok := historicalContextMessage(historical); ok {
			optional = append(optional, ContextUnit{Source: ContextHistoricalEvidence, Timestamp: newestHistoricalTimestamp(historical), Priority: 70, Messages: []Message{historicalMessage}})
		}
		optional = append(optional, recentContextUnits(recent)...)
	}
	filteredTools := cloneAnySlice(filterTools(tools, inv.Mode(), inv.Prompt()))
	pr, e := ProjectTurn(nil, filteredTools, nil, ContextInput{
		RequiredPrefix: []Message{llm.NewLlmMessage("system", sys, nil, "")},
		Optional:       optional,
		RequiredSuffix: []Message{llm.NewLlmMessage("user", promptText, nil, "")},
		MaxTokens:      maxTokens,
		ReserveTokens:  a.config.ContextReserveTokens,
	})
	if e != nil {
		return PreparedRequest{}, e
	}
	return PreparedRequest{messages: pr.Messages, tools: filteredTools, contextualized: promptText, kind: kind, projection: pr}, nil
}

// ProjectTurn reapplies this assembler's configured limits to an evolving
// request transcript. newestRequest is the immutable contextualized request
// produced during initial assembly.
func (a *Assembler) ProjectTurn(messages []Message, tools []any, observations *ObservationStore, newestRequest Message) (Projection, error) {
	if a == nil || a.system == nil {
		return Projection{}, errors.New("assembler is not initialized")
	}
	var policy Message
	foundPolicy := false
	for _, message := range messages {
		if message.Role() == "system" {
			policy = message
			foundPolicy = true
			break
		}
	}
	if !foundPolicy || newestRequest.Role() != "user" {
		return Projection{}, errors.New("turn projection requires policy and newest request")
	}
	foundNewest := false
	for _, message := range messages {
		if messageFingerprint(message) == messageFingerprint(newestRequest) {
			foundNewest = true
			break
		}
	}
	if !foundNewest {
		return Projection{}, errors.New("turn projection transcript lost newest request")
	}
	return ProjectTurn(messages, tools, observations, ContextInput{
		RequiredPrefix: []Message{policy},
		RequiredSuffix: []Message{newestRequest},
		MaxTokens:      a.maxContextTokens(),
		ReserveTokens:  a.config.ContextReserveTokens,
	})
}

func (a *Assembler) maxContextTokens() int {
	maxTokens := a.config.MaxContextTokens
	if maxTokens > 0 {
		return maxTokens
	}
	budgetChars := 32000
	if a.config.MaxPromptChars > 0 && a.config.MaxPromptChars <= 500000 && a.config.MaxPromptChars*8 > budgetChars {
		budgetChars = a.config.MaxPromptChars * 8
	}
	return (budgetChars + 3) / 4
}

func historyContextUnits(history []Message) []ContextUnit {
	units := make([]ContextUnit, 0, len(history))
	for index := 0; index < len(history); index++ {
		message := history[index]
		if isInternalToolEvidence(message.Content()) || message.Role() == "system" {
			continue
		}
		unit := []Message{llm.NewLlmMessage(message.Role(), message.Content(), message.ToolCalls(), message.ToolCallID())}
		if len(message.ToolCalls()) > 0 {
			ids := make(map[string]bool, len(message.ToolCalls()))
			for _, call := range message.ToolCalls() {
				ids[call.ID()] = true
			}
			for index+1 < len(history) && history[index+1].Role() == "tool" && ids[history[index+1].ToolCallID()] {
				index++
				toolMessage := history[index]
				unit = append(unit, llm.NewLlmMessage(toolMessage.Role(), toolMessage.Content(), toolMessage.ToolCalls(), toolMessage.ToolCallID()))
			}
		}
		units = append(units, ContextUnit{Source: ContextMemory, Priority: 50, Messages: unit})
	}
	return units
}

const recentContextChunkBytes = 2048

type recentRoomRows struct {
	Rows []json.RawMessage `json:"rows"`
}

func recentContextUnits(recent string) []ContextUnit {
	recent = strings.TrimSpace(recent)
	if recent == "" || !json.Valid([]byte(recent)) {
		return nil
	}
	var envelope recentRoomRows
	if err := json.Unmarshal([]byte(recent), &envelope); err != nil || envelope.Rows == nil {
		return []ContextUnit{recentContextUnit(recent, 0)}
	}
	units := make([]ContextUnit, 0, len(envelope.Rows))
	chunk := make([]json.RawMessage, 0)
	for _, row := range envelope.Rows {
		candidate := append(append([]json.RawMessage(nil), chunk...), row)
		encoded, ok := encodeRecentRows(candidate)
		if !ok {
			continue
		}
		if len(chunk) > 0 && len(encoded) > recentContextChunkBytes {
			units = append(units, recentContextUnitFromRows(chunk))
			chunk = []json.RawMessage{row}
			continue
		}
		chunk = candidate
	}
	if len(chunk) > 0 {
		units = append(units, recentContextUnitFromRows(chunk))
	}
	return units
}

func recentContextUnitFromRows(rows []json.RawMessage) ContextUnit {
	encoded, _ := encodeRecentRows(rows)
	var timestamp int64
	for _, row := range rows {
		var metadata struct {
			CreatedOn int64 `json:"createdOn"`
		}
		if json.Unmarshal(row, &metadata) == nil && metadata.CreatedOn > timestamp {
			timestamp = metadata.CreatedOn
		}
	}
	return recentContextUnit(encoded, timestamp)
}

func recentContextUnit(payload string, timestamp int64) ContextUnit {
	message := llm.NewLlmMessage("user", "RECENT_PUBLIC_ROOM_MESSAGES_UNTRUSTED_DATA="+payload, nil, "")
	return ContextUnit{Source: ContextRecentRoom, Timestamp: timestamp, Priority: 80, Messages: []Message{message}}
}

func encodeRecentRows(rows []json.RawMessage) (string, bool) {
	payload, err := json.Marshal(recentRoomRows{Rows: rows})
	if err != nil {
		return "", false
	}
	return string(payload), true
}

func historicalContextMessage(historical []turn.HistoricalEvidence) (Message, bool) {
	items := make([]map[string]any, 0, len(historical))
	for _, item := range historical {
		if strings.TrimSpace(item.Tool) == "" || item.ObservedAtMillis < 0 || len([]byte(item.Content)) > 32000 || !json.Valid([]byte(item.Content)) {
			continue
		}
		var data any
		if json.Unmarshal([]byte(item.Content), &data) != nil {
			continue
		}
		items = append(items, map[string]any{"tool": item.Tool, "observedAtMillis": item.ObservedAtMillis, "data": data})
	}
	if len(items) == 0 {
		return Message{}, false
	}
	payload, err := json.Marshal(map[string]any{"historicalToolEvidence": items})
	if err != nil {
		return Message{}, false
	}
	return llm.NewLlmMessage("user", "HISTORICAL_TOOL_EVIDENCE_UNTRUSTED_DATA="+string(payload)+"\nThis evidence may be stale; current tool observations supersede it.", nil, ""), true
}

func newestHistoricalTimestamp(historical []turn.HistoricalEvidence) int64 {
	var newest int64
	for _, item := range historical {
		if item.ObservedAtMillis > newest {
			newest = item.ObservedAtMillis
		}
	}
	return newest
}

func isInternalToolEvidence(content string) bool {
	return strings.HasPrefix(strings.TrimSpace(content), "[Internal tool evidence from ")
}

func cloneAnySlice(in []any) []any {
	out := make([]any, len(in))
	for i, value := range in {
		out[i] = cloneValue(value)
	}
	return out
}

func cloneValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, nested := range value {
			out[key] = cloneValue(nested)
		}
		return out
	case []any:
		return cloneAnySlice(value)
	default:
		return value
	}
}
func filterTools(in []any, mode runtime.Mode, prompt string) []any {
	out := []any{}
	for _, v := range in {
		name := toolName(v)
		if mode == runtime.MODERATION && !participation.IsSemanticModerationTool(name) {
			continue
		}
		if mode == runtime.AMBIENT && strings.HasPrefix(name, "saturn_") {
			continue
		}
		out = append(out, v)
	}
	return out
}
func toolName(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if f, ok := m["function"].(map[string]any); ok {
		if s, ok := f["name"].(string); ok {
			return s
		}
	}
	if s, ok := m["name"].(string); ok {
		return s
	}
	return ""
}

// Truncate is Unicode-safe and treats nil-like input as empty text.
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i]
		}
		n++
	}
	return s
}
func CodePointCount(s string) int { return utf8.RuneCountInString(s) }
