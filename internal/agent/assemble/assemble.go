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
	"zenbot/internal/agent/runtime"
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
}

// RequestKind identifies the trusted classification metadata carried in the system prompt.
type RequestKind string

const (
	Unclassified RequestKind = "UNCLASSIFIED"
	Talk         RequestKind = "TALK"
	Command      RequestKind = "COMMAND"
)

// ToolEvidence records trusted tool-loop evidence for prompt metadata.
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
func (p *SystemPrompt) Render(inv runtime.Invocation, correlationID, recent string, kind RequestKind, evidence ToolEvidence, phase string) (string, error) {
	if p == nil || p.catalog == nil {
		return "", errors.New("prompt catalog must not be nil")
	}
	ctx := inv.Context()
	caller := map[string]any{"nick": ctx.Nick(), "trip": ctx.Trip(), "hash": ctx.Hash(), "creator": p.config.CreatorTrip != "" && p.config.CreatorTrip == ctx.Trip()}
	runtimeMeta := map[string]any{"correlationId": correlationID, "invocationMode": string(inv.Mode()), "requestKind": string(kind), "requestKindPhase": phase, "toolEvidence": map[string]any{"attempted": evidence.Attempted, "attemptedCount": evidence.AttemptedCount, "successfulCount": evidence.SuccessfulCount, "failedCount": evidence.FailedCount}, "room": ctx.Room(), "whisper": ctx.Whisper(), "caller": caller, "roomUsersSnapshot": ctx.RoomUsers()}
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
	room := strings.TrimSpace(recent)
	if room == "" {
		room = `{"rows":[]}`
	}
	return p.catalog.Formatted("system/system-policy.txt", p.config.CreatorTrip, map[bool]string{true: "private whisper", false: "shared room"}[ctx.Whisper()], database, participation, persona, string(meta), room)
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
	n := 0
	for _, m := range ms {
		n += len([]byte(m.Role() + "|" + m.Content() + "|" + m.ToolCallID() + fmt.Sprint(m.ToolCalls())))
	}
	return n
}
func fingerprint(ms []Message) string {
	h := sha256.New()
	for _, m := range ms {
		h.Write([]byte(m.Role() + "|" + m.Content() + "|" + m.ToolCallID() + fmt.Sprint(m.ToolCalls())))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// PreparedRequest is immutable request state prepared before provider execution.
type PreparedRequest struct {
	messages                   []Message
	tools                      []any
	contextualized             string
	requiredTool, requiredNick string
	kind                       RequestKind
	projection                 Projection
}

func (r PreparedRequest) Messages() []Message          { return append([]Message(nil), r.messages...) }
func (r PreparedRequest) Tools() []any                 { return cloneAnySlice(r.tools) }
func (r PreparedRequest) ContextualizedPrompt() string { return r.contextualized }
func (r PreparedRequest) RequiredFreshTool() string    { return r.requiredTool }
func (r PreparedRequest) RequiredFreshNick() string    { return r.requiredNick }
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
	if err := ctx.Err(); err != nil {
		return PreparedRequest{}, err
	}
	if a == nil || a.system == nil {
		return PreparedRequest{}, errors.New("assembler is not initialized")
	}
	budget := 32000
	maxInt := int(^uint(0) >> 1)
	if a.config.MaxPromptChars > maxInt/8 {
		budget = maxInt
	} else if a.config.MaxPromptChars > 0 && a.config.MaxPromptChars*8 > budget {
		budget = a.config.MaxPromptChars * 8
	}
	recent = Truncate(recent, budget)
	sys, e := a.system.Render(inv, inv.RequestID(), recent, kind, ToolEvidence{}, "CANDIDATE")
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
	ms := []Message{llm.NewLlmMessage("system", sys, nil, "")}
	for _, message := range history {
		if !isInternalToolEvidence(message.Content()) {
			ms = append(ms, llm.NewLlmMessage(message.Role(), message.Content(), message.ToolCalls(), message.ToolCallID()))
		}
	}
	ms = append(ms, llm.NewLlmMessage("user", promptText, nil, ""))
	pr := project(ms, budget)
	freshTool, freshNick := freshness(inv.Prompt(), history, ctxr.RoomUsers())
	if inv.Mode() == runtime.MODERATION {
		freshTool, freshNick = "", ""
	}
	return PreparedRequest{messages: pr.Messages, tools: cloneAnySlice(filterTools(tools, inv.Mode(), inv.Prompt())), contextualized: promptText, requiredTool: freshTool, requiredNick: freshNick, kind: kind, projection: pr}, nil
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
		if mode == runtime.MODERATION && name != "run_command" {
			continue
		}
		if mode != runtime.MODERATION && strings.HasPrefix(name, "saturn_") {
			a := strings.TrimPrefix(name, "saturn_")
			toks := strings.Fields(prompt)
			ok := len(toks) > 0 && toks[0] == a || len(toks) > 1 && (toks[0] == "run" || toks[0] == "execute") && toks[1] == a
			if !ok {
				continue
			}
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
func freshness(prompt string, history []Message, users []string) (string, string) {
	p := strings.TrimSpace(strings.ReplaceAll(prompt, "\\_", "_"))
	lower := strings.ToLower(p)
	words := strings.Fields(lower)
	for i, w := range words {
		if w == "user" && i > 0 {
			// “tell me about jill user” is Saturn’s trailing-target form.
			return "user_message_history", strings.Trim(strings.Trim(words[i-1], "?.!,:'\""), "@")
		}
		if w == "is" && i+1 < len(words) {
			return "user_message_history", strings.Trim(strings.Trim(words[i+1], "?.!,:'\""), "@")
		}
	}
	if strings.HasPrefix(lower, "tell me about ") {
		target := strings.TrimSpace(p[len("tell me about "):])
		fields := strings.Fields(target)
		if len(fields) == 1 {
			return "user_message_history", strings.Trim(fields[0], "?.!,:'\"@")
		}
	}
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role() == "user" && (lower == "do it" || lower == "check it") {
			return freshness(history[i].Content(), nil, users)
		}
	}
	return "", ""
}
