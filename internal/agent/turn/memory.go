package turn

import (
	"errors"

	"strings"
	"sync"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
)

var ErrInvalidEvidence = errors.New("invalid tool evidence")
var ErrMemoryLoad = errors.New("Agent memory load failed")
var ErrMemoryPersistence = errors.New("Agent memory persistence failed")

type EvidenceEntry struct{ Tool, Content string }
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
	h, err := m.store.Load(ctx)
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
func (m TurnMemory) Append(ctx api.Context, user, assistant, _ string) error {
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
