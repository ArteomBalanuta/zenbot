package live

import (
	"context"
	"fmt"
	"strings"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/turn"
	"zenbot/internal/repository"
)

// PersistentMemoryStore adapts the narrow repository contract without shared pending state.
type PersistentMemoryStore struct {
	Repository             repository.AgentMemoryRepository
	ToolEvidenceRepository repository.AgentToolEvidenceRepository
	Turns                  int
	TTL                    time.Duration
	Clock                  func() time.Time
	Compactor              *turn.MemoryCompactor
	RawTurnLimit           int
}

func (s PersistentMemoryStore) Load(agent api.Context) ([]llm.LlmMessage, error) {
	return s.LoadContext(context.Background(), agent)
}
func (s PersistentMemoryStore) LoadContext(ctx context.Context, agent api.Context) ([]llm.LlmMessage, error) {
	if s.Repository == nil || s.Turns < 1 || s.TTL <= 0 {
		return nil, fmt.Errorf("memory store is not configured")
	}
	now := s.now().UnixMilli()
	rows, err := s.Repository.LoadAgentMemory(ctx, agent.MemoryKey(), now, s.Turns)
	if err != nil {
		return nil, err
	}
	var existing *turn.MemorySummary
	var summaryRecord *repository.AgentMemorySummary
	var summaryRepository repository.AgentMemorySummaryRepository
	if s.Compactor != nil {
		if s.RawTurnLimit < 1 {
			return nil, fmt.Errorf("memory compaction raw turn limit is invalid")
		}
		var ok bool
		summaryRepository, ok = s.Repository.(repository.AgentMemorySummaryRepository)
		if !ok {
			return nil, fmt.Errorf("memory summary repository is not configured")
		}
		summaryRecord, err = summaryRepository.LoadAgentMemorySummary(ctx, agent.MemoryKey(), now)
		if err != nil {
			return nil, err
		}
		if summaryRecord != nil {
			existing = &turn.MemorySummary{Content: summaryRecord.Content, Fingerprint: summaryRecord.Fingerprint, SourceCursor: summaryRecord.CoveredThroughID}
		}
	}
	out := make([]llm.LlmMessage, 0, len(rows))
	eligibleRows := make([]repository.AgentMemoryMessage, 0, len(rows))
	for _, row := range rows {
		if existing != nil && row.ID > 0 && row.ID <= existing.SourceCursor {
			continue
		}
		if (row.Role != "user" && row.Role != "assistant") || strings.TrimSpace(row.Content) == "" {
			return nil, fmt.Errorf("invalid memory row")
		}
		eligibleRows = append(eligibleRows, row)
		out = append(out, llm.NewLlmMessage(row.Role, row.Content, nil, ""))
	}
	if s.Compactor == nil {
		return out, nil
	}
	compacted, err := s.Compactor.Compact(ctx, turn.CompactionInput{Messages: out, RawTurnLimit: s.RawTurnLimit, ExistingSummary: existing})
	if err != nil {
		return nil, err
	}
	if compacted.Summary != nil && (summaryRecord == nil || compacted.Summary.Fingerprint != summaryRecord.Fingerprint) {
		compactedCount := len(out) - (len(compacted.Messages) - 1)
		if compactedCount < 1 || compactedCount > len(eligibleRows) || eligibleRows[compactedCount-1].ID < 1 {
			return nil, fmt.Errorf("memory compaction coverage is invalid")
		}
		compacted.Summary.SourceCursor = eligibleRows[compactedCount-1].ID
		created := s.now()
		if err := summaryRepository.UpsertAgentMemorySummary(ctx, repository.AgentMemorySummary{
			IdentityKey:      agent.MemoryKey(),
			Content:          compacted.Summary.Content,
			CoveredThroughID: compacted.Summary.SourceCursor,
			Fingerprint:      compacted.Summary.Fingerprint,
			CreatedOnMillis:  created.UnixMilli(),
			ExpiresOnMillis:  created.Add(s.TTL).UnixMilli(),
		}); err != nil {
			return nil, err
		}
	}
	return compacted.Messages, nil
}
func (s PersistentMemoryStore) Append(agent api.Context, role, content string) error {
	return fmt.Errorf("individual durable memory append is unsupported")
}
func (s PersistentMemoryStore) AppendToolEvidence(agent api.Context, toolName, content string) error {
	return s.AppendEvidenceContext(context.Background(), agent, []turn.EvidenceEntry{{Tool: toolName, Content: content}})
}
func (s PersistentMemoryStore) AppendEvidenceContext(ctx context.Context, agent api.Context, rows []turn.EvidenceEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if agent.Whisper() || s.ToolEvidenceRepository == nil || s.TTL <= 0 {
		return fmt.Errorf("tool evidence store is not configured")
	}
	now := s.now()
	for _, row := range rows {
		if strings.TrimSpace(row.Tool) == "" || strings.TrimSpace(row.Content) == "" {
			return fmt.Errorf("tool evidence is invalid")
		}
		if err := s.ToolEvidenceRepository.AppendAgentToolEvidence(ctx, agent.MemoryKey(), row.Tool, row.Content, now.UnixMilli(), now.Add(s.TTL).UnixMilli()); err != nil {
			return err
		}
	}
	return nil
}
func (s PersistentMemoryStore) LoadToolEvidenceContext(ctx context.Context, agent api.Context) ([]repository.AgentToolEvidence, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if agent.Whisper() {
		return []repository.AgentToolEvidence{}, nil
	}
	if s.ToolEvidenceRepository == nil || s.Turns < 1 || s.TTL <= 0 {
		return nil, fmt.Errorf("tool evidence store is not configured")
	}
	return s.ToolEvidenceRepository.LoadAgentToolEvidence(ctx, agent.MemoryKey(), s.now().UnixMilli(), s.Turns)
}
func (s PersistentMemoryStore) AppendExchange(agent api.Context, user, assistant string) error {
	return s.AppendExchangeContext(context.Background(), agent, user, assistant)
}
func (s PersistentMemoryStore) AppendExchangeContext(ctx context.Context, agent api.Context, user, assistant string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Repository == nil || s.TTL <= 0 || strings.TrimSpace(user) == "" || strings.TrimSpace(assistant) == "" {
		return fmt.Errorf("memory exchange is invalid")
	}
	now := s.now()
	return s.Repository.AppendAgentMemory(ctx, agent.MemoryKey(), user, assistant, now.UnixMilli(), now.Add(s.TTL).UnixMilli())
}
func (s PersistentMemoryStore) AppendTurnContext(ctx context.Context, agent api.Context, user, assistant string, evidence []turn.PersistableEvidence) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	turnRepository, ok := s.Repository.(repository.AgentTurnRepository)
	if !ok || s.TTL <= 0 || strings.TrimSpace(user) == "" || strings.TrimSpace(assistant) == "" {
		return fmt.Errorf("atomic memory store is not configured")
	}
	now := s.now()
	record := repository.AgentTurnRecord{IdentityKey: agent.MemoryKey(), User: user, Assistant: assistant, CreatedOnMillis: now.UnixMilli(), ExpiresOnMillis: now.Add(s.TTL).UnixMilli()}
	if !agent.Whisper() {
		record.Evidence = make([]repository.AgentTurnEvidence, len(evidence))
		for index, item := range evidence {
			record.Evidence[index] = repository.AgentTurnEvidence{ToolName: item.Tool, Content: item.Content}
		}
	}
	return turnRepository.AppendAgentTurn(ctx, record)
}
func (s PersistentMemoryStore) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}
