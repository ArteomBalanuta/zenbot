package turn

import (
	"context"
	"encoding/json"
	"errors"

	"strings"
	"sync"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/repository"
)

var ErrInvalidEvidence = errors.New("invalid tool evidence")
var ErrMemoryLoad = errors.New("Agent memory load failed")
var ErrMemoryPersistence = errors.New("Agent memory persistence failed")

type EvidenceEntry struct{ Tool, Content string }
type PersistableEvidence struct{ Tool, Content string }

// HistoricalEvidence is validated durable tool data for one later public request.
type HistoricalEvidence struct {
	Tool             string
	Content          string
	ObservedAtMillis int64
}

const maxDurableEvidenceBytes = 32000

var durableEvidenceSchemas = map[string]json.RawMessage{
	"user_message_history": contract.SchemaObject(map[string]json.RawMessage{
		"rows":          json.RawMessage(`{"type":"array"}`),
		"returnedCount": json.RawMessage(`{"type":"integer"}`),
	}, []string{"rows", "returnedCount"}, false),
	"room_users": contract.SchemaObject(map[string]json.RawMessage{
		"room":          json.RawMessage(`{"type":"string"}`),
		"users":         json.RawMessage(`{"type":"array","items":{"type":"string"}}`),
		"count":         json.RawMessage(`{"type":"integer"}`),
		"returnedCount": json.RawMessage(`{"type":"integer"}`),
		"truncated":     json.RawMessage(`{"type":"boolean"}`),
	}, []string{"room", "users", "count", "returnedCount", "truncated"}, false),
}

func validPersistableEvidence(e PersistableEvidence) bool {
	schema, ok := durableEvidenceSchemas[e.Tool]
	return ok && strings.TrimSpace(e.Content) != "" && len([]byte(e.Content)) <= maxDurableEvidenceBytes && json.Valid([]byte(e.Content)) && contract.ValidateResult(schema, []byte(e.Content)) == nil
}

func NewPersistableEvidence(d contract.Descriptor, result contract.Result) (PersistableEvidence, error) {
	candidate := PersistableEvidence{Tool: d.Name(), Content: result.Content}
	if result.IsError || d.ResultMode() != contract.ModelData || !d.IsReadOnly() || !d.Idempotent() || len(d.ResourceWrites()) != 0 || len(d.ResourceReads()) == 0 || d.Name() != result.ToolName || !validPersistableEvidence(candidate) || contract.ValidateResult(d.ResultSchema(), []byte(result.Content)) != nil {
		return PersistableEvidence{}, ErrInvalidEvidence
	}
	return PersistableEvidence{Tool: candidate.Tool, Content: string(append([]byte(nil), candidate.Content...))}, nil
}

type MemoryStore interface {
	Load(api.Context) ([]llm.LlmMessage, error)
	Append(api.Context, string, string) error
	AppendToolEvidence(api.Context, string, string) error
}
type memoryData struct {
	messages []llm.LlmMessage
	evidence []EvidenceEntry
}
type InMemoryStore struct {
	mu      sync.Mutex
	data    map[string]memoryData
	failure error
}

func NewMemoryStore() *InMemoryStore                       { return &InMemoryStore{data: map[string]memoryData{}} }
func (m *InMemoryStore) bucket(ctx api.Context) memoryData { return m.data[ctx.MemoryKey()] }
func (m *InMemoryStore) put(ctx api.Context, d memoryData) { m.data[ctx.MemoryKey()] = d }
func (m *InMemoryStore) Load(ctx api.Context) ([]llm.LlmMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure != nil {
		return nil, ErrMemoryLoad
	}
	out := []llm.LlmMessage{}
	for _, x := range m.bucket(ctx).messages {
		if stringsLegacy(x.Content()) || IsInternalToolEvidence(x.Content()) {
			continue
		}
		out = append(out, x)
	}
	return out, nil
}
func (m *InMemoryStore) Append(ctx api.Context, role, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure != nil {
		return ErrMemoryPersistence
	}
	d := m.bucket(ctx)
	d.messages = append(d.messages, llm.NewLlmMessage(role, content, nil, ""))
	m.put(ctx, d)
	return nil
}
func (m *InMemoryStore) AppendToolEvidence(ctx api.Context, tool, content string) error {
	if strings.TrimSpace(tool) == "" || strings.TrimSpace(content) == "" {
		return ErrInvalidEvidence
	}
	return m.appendEvidence(ctx, []EvidenceEntry{{tool, content}})
}
func (m *InMemoryStore) AppendEvidence(es []EvidenceEntry) error {
	return m.appendEvidence(api.Context{}, es)
}
func (m *InMemoryStore) AppendEvidenceFor(ctx api.Context, es []EvidenceEntry) error {
	return m.appendEvidence(ctx, es)
}
func (m *InMemoryStore) appendEvidence(ctx api.Context, es []EvidenceEntry) error {
	for _, e := range es {
		if strings.TrimSpace(e.Tool) == "" || strings.TrimSpace(e.Content) == "" {
			return ErrInvalidEvidence
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure != nil {
		return ErrMemoryPersistence
	}
	d := m.bucket(ctx)
	d.evidence = append(d.evidence, es...)
	m.put(ctx, d)
	return nil
}
func (m *InMemoryStore) Evidence() []EvidenceEntry {
	return m.EvidenceFor(api.Context{})
}
func (m *InMemoryStore) EvidenceFor(ctx api.Context) []EvidenceEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]EvidenceEntry(nil), m.bucket(ctx).evidence...)
}

// TurnMemory is the narrow request-local facade; it deliberately has no persistence policy.
type TurnMemory struct{ store MemoryStore }

func NewTurnMemory(store MemoryStore) (TurnMemory, error) {
	if store == nil {
		return TurnMemory{}, errors.New("memory store missing")
	}
	return TurnMemory{store: store}, nil
}
func (m TurnMemory) Load(ctx api.Context, _ string) ([]llm.LlmMessage, error) {
	return m.LoadContext(context.Background(), ctx, "")
}
func (m TurnMemory) LoadContext(parent context.Context, ctx api.Context, _ string) ([]llm.LlmMessage, error) {
	if err := parent.Err(); err != nil {
		return nil, err
	}
	var h []llm.LlmMessage
	var err error
	if loader, ok := m.store.(interface {
		LoadContext(context.Context, api.Context) ([]llm.LlmMessage, error)
	}); ok {
		h, err = loader.LoadContext(parent, ctx)
	} else {
		h, err = m.store.Load(ctx)
	}
	if err != nil {
		return nil, memoryError{sentinel: ErrMemoryLoad, cause: err}
	}
	if h == nil {
		return nil, memoryError{sentinel: ErrMemoryLoad, cause: errors.New("nil history")}
	}
	out := make([]llm.LlmMessage, 0, len(h))
	for _, msg := range h {
		if stringsLegacy(msg.Content()) || IsInternalToolEvidence(msg.Content()) {
			if stringsLegacy(msg.Content()) && len(out) > 0 && out[len(out)-1].Role() == "user" {
				out = out[:len(out)-1]
			}
			continue
		}
		out = append(out, msg)
	}
	return out, nil
}

// LoadHistoricalEvidenceContext loads bounded structured durable evidence only for public requests.
func (m TurnMemory) LoadHistoricalEvidenceContext(parent context.Context, ctx api.Context) ([]HistoricalEvidence, error) {
	if err := parent.Err(); err != nil {
		return nil, err
	}
	if ctx.Whisper() {
		return []HistoricalEvidence{}, nil
	}
	loader, ok := m.store.(interface {
		LoadToolEvidenceContext(context.Context, api.Context) ([]repository.AgentToolEvidence, error)
	})
	if !ok {
		return []HistoricalEvidence{}, nil
	}
	rows, err := loader.LoadToolEvidenceContext(parent, ctx)
	if err != nil {
		return nil, memoryError{sentinel: ErrMemoryLoad, cause: err}
	}
	out := make([]HistoricalEvidence, 0, len(rows))
	for _, row := range rows {
		candidate := PersistableEvidence{Tool: row.ToolName, Content: row.Content}
		if row.CreatedOnMillis < 0 || !validPersistableEvidence(candidate) {
			continue
		}
		out = append(out, HistoricalEvidence{Tool: candidate.Tool, Content: string(append([]byte(nil), candidate.Content...)), ObservedAtMillis: row.CreatedOnMillis})
	}
	return out, nil
}
func (m TurnMemory) Append(ctx api.Context, user, assistant, correlationID string) error {
	return m.AppendContext(context.Background(), ctx, user, assistant, correlationID)
}
func (m TurnMemory) AppendContext(parent context.Context, ctx api.Context, user, assistant, _ string) error {
	if err := parent.Err(); err != nil {
		return err
	}
	if exchange, ok := m.store.(interface {
		AppendExchangeContext(context.Context, api.Context, string, string) error
	}); ok {
		if err := exchange.AppendExchangeContext(parent, ctx, user, assistant); err != nil {
			return memoryError{sentinel: ErrMemoryPersistence, cause: err}
		}
		return nil
	}
	if exchange, ok := m.store.(interface {
		AppendExchange(api.Context, string, string) error
	}); ok {
		if err := exchange.AppendExchange(ctx, user, assistant); err != nil {
			return memoryError{sentinel: ErrMemoryPersistence, cause: err}
		}
		return nil
	}
	if err := m.store.Append(ctx, "user", user); err != nil {
		return memoryError{sentinel: ErrMemoryPersistence, cause: err}
	}
	if err := m.store.Append(ctx, "assistant", assistant); err != nil {
		return memoryError{sentinel: ErrMemoryPersistence, cause: err}
	}
	return nil
}
func (m TurnMemory) AppendToolEvidence(ctx api.Context, es []EvidenceEntry, _ string) error {
	for _, e := range es {
		if strings.TrimSpace(e.Tool) == "" || strings.TrimSpace(e.Content) == "" {
			return ErrInvalidEvidence
		}
	}
	if len(es) == 0 {
		return nil
	}
	if batch, ok := m.store.(interface {
		AppendEvidenceFor(api.Context, []EvidenceEntry) error
	}); ok {
		if err := batch.AppendEvidenceFor(ctx, es); err != nil {
			return memoryError{sentinel: ErrMemoryPersistence, cause: err}
		}
		return nil
	}
	for _, e := range es {
		if err := m.store.AppendToolEvidence(ctx, e.Tool, e.Content); err != nil {
			return memoryError{sentinel: ErrMemoryPersistence, cause: err}
		}
	}
	return nil
}
func (m TurnMemory) AppendToolEvidenceContext(parent context.Context, ctx api.Context, es []PersistableEvidence) error {
	if err := parent.Err(); err != nil {
		return err
	}
	if len(es) == 0 {
		return nil
	}
	entries := make([]EvidenceEntry, len(es))
	for i, e := range es {
		if !validPersistableEvidence(e) {
			return ErrInvalidEvidence
		}
		entries[i] = EvidenceEntry{Tool: e.Tool, Content: string(append([]byte(nil), e.Content...))}
	}
	if batch, ok := m.store.(interface {
		AppendEvidenceContext(context.Context, api.Context, []EvidenceEntry) error
	}); ok {
		if err := batch.AppendEvidenceContext(parent, ctx, entries); err != nil {
			return memoryError{sentinel: ErrMemoryPersistence, cause: err}
		}
		return nil
	}
	return m.AppendToolEvidence(ctx, entries, "")
}

type memoryError struct{ sentinel, cause error }

func (e memoryError) Error() string        { return e.sentinel.Error() }
func (e memoryError) Unwrap() error        { return e.cause }
func (e memoryError) Is(target error) bool { return target == e.sentinel || errors.Is(e.cause, target) }

func stringsLegacy(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "[legacy persona]" || lower == "legacy persona" || strings.Contains(lower, "[sips tea") || strings.Contains(lower, "the archives reveal") {
		return true
	}
	for _, line := range strings.Split(lower, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "carpe diem") || strings.HasPrefix(line, "your history shows:") {
			return true
		}
	}
	return false
}
