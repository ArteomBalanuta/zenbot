package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"zenbot/internal/repository"
)

const loadAgentMemorySQL = `SELECT id, role, content, created_on FROM (
  SELECT id, role, content, created_on FROM agent_memory
  WHERE identity_key = ?1 AND expires_on > ?2 AND role IN ('user', 'assistant')
  ORDER BY created_on DESC, id DESC LIMIT ?3
) recent ORDER BY created_on ASC, id ASC`

const loadAgentToolEvidenceSQL = `SELECT tool_name, content, created_on FROM (
  SELECT id, tool_name, content, created_on FROM agent_tool_memory
  WHERE identity_key = ?1 AND expires_on > ?2
  ORDER BY created_on DESC, id DESC LIMIT ?3
) recent ORDER BY created_on ASC, id ASC`

func (d *Database) LoadAgentMemory(ctx context.Context, key string, nowMillis int64, turns int) ([]repository.AgentMemoryMessage, error) {
	if d == nil || d.DB == nil {
		return nil, fmt.Errorf("load agent memory: database is not initialized")
	}
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("load agent memory: key is required")
	}
	if nowMillis < 0 {
		return nil, fmt.Errorf("load agent memory: clock must not be negative")
	}
	if turns < 1 {
		return nil, fmt.Errorf("load agent memory: turns must be positive")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load agent memory: %w", err)
	}
	rows, err := d.DB.QueryContext(ctx, loadAgentMemorySQL, key, nowMillis, strconv.Itoa(turns*2))
	if err != nil {
		return nil, fmt.Errorf("load agent memory: %w", err)
	}
	defer rows.Close()
	out := []repository.AgentMemoryMessage{}
	for rows.Next() {
		var message repository.AgentMemoryMessage
		if err := rows.Scan(&message.ID, &message.Role, &message.Content, &message.CreatedOnMillis); err != nil {
			return nil, fmt.Errorf("load agent memory: %w", err)
		}
		if message.Role != "user" && message.Role != "assistant" || strings.TrimSpace(message.Content) == "" {
			return nil, fmt.Errorf("load agent memory: invalid stored message")
		}
		out = append(out, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load agent memory: %w", err)
	}
	return out, nil
}

func (d *Database) LoadAgentMemorySummary(ctx context.Context, key string, nowMillis int64) (*repository.AgentMemorySummary, error) {
	if d == nil || d.DB == nil {
		return nil, fmt.Errorf("load agent memory summary: database is not initialized")
	}
	if strings.TrimSpace(key) == "" || nowMillis < 0 {
		return nil, fmt.Errorf("load agent memory summary: key and clock are invalid")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load agent memory summary: %w", err)
	}
	row := repository.AgentMemorySummary{IdentityKey: key}
	err := d.DB.QueryRowContext(ctx, `SELECT content, covered_through_id, fingerprint, created_on, expires_on FROM agent_memory_summary WHERE identity_key = ?1 AND expires_on > ?2`, key, nowMillis).Scan(&row.Content, &row.CoveredThroughID, &row.Fingerprint, &row.CreatedOnMillis, &row.ExpiresOnMillis)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load agent memory summary: %w", err)
	}
	if strings.TrimSpace(row.Content) == "" || row.CoveredThroughID < 1 || strings.TrimSpace(row.Fingerprint) == "" || row.ExpiresOnMillis <= row.CreatedOnMillis {
		return nil, fmt.Errorf("load agent memory summary: invalid stored summary")
	}
	return &row, nil
}

func (d *Database) UpsertAgentMemorySummary(ctx context.Context, summary repository.AgentMemorySummary) error {
	if d == nil || d.DB == nil {
		return fmt.Errorf("upsert agent memory summary: database is not initialized")
	}
	if strings.TrimSpace(summary.IdentityKey) == "" || strings.TrimSpace(summary.Content) == "" || summary.CoveredThroughID < 1 || strings.TrimSpace(summary.Fingerprint) == "" || summary.ExpiresOnMillis <= summary.CreatedOnMillis {
		return fmt.Errorf("upsert agent memory summary: invalid summary")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("upsert agent memory summary: %w", err)
	}
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("upsert agent memory summary: %w", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM agent_memory_summary WHERE identity_key = ?1`, summary.IdentityKey); err != nil {
		return fmt.Errorf("upsert agent memory summary delete: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_memory_summary(identity_key, content, covered_through_id, fingerprint, created_on, expires_on) VALUES (?1,?2,?3,?4,?5,?6)`, summary.IdentityKey, summary.Content, summary.CoveredThroughID, summary.Fingerprint, summary.CreatedOnMillis, summary.ExpiresOnMillis); err != nil {
		return fmt.Errorf("upsert agent memory summary insert: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("upsert agent memory summary commit: %w", err)
	}
	return nil
}

func (d *Database) AppendAgentMemory(ctx context.Context, key, user, assistant string, createdOnMillis, expiresOnMillis int64) error {
	if d == nil || d.DB == nil {
		return fmt.Errorf("append agent memory: database is not initialized")
	}
	if strings.TrimSpace(key) == "" || strings.TrimSpace(user) == "" || strings.TrimSpace(assistant) == "" {
		return fmt.Errorf("append agent memory: key and exchange content are required")
	}
	if expiresOnMillis <= createdOnMillis {
		return fmt.Errorf("append agent memory: expiry must be after creation")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("append agent memory: %w", err)
	}
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("append agent memory: %w", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM agent_memory WHERE expires_on <= ?1`, createdOnMillis); err != nil {
		return fmt.Errorf("append agent memory cleanup: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO agent_memory(identity_key, role, content, created_on, expires_on) VALUES (?1,?2,?3,?4,?5)`)
	if err != nil {
		return fmt.Errorf("append agent memory prepare: %w", err)
	}
	defer stmt.Close()
	if _, err = stmt.ExecContext(ctx, key, "user", user, createdOnMillis, expiresOnMillis); err != nil {
		return fmt.Errorf("append agent memory user: %w", err)
	}
	if _, err = stmt.ExecContext(ctx, key, "assistant", assistant, createdOnMillis, expiresOnMillis); err != nil {
		return fmt.Errorf("append agent memory assistant: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("append agent memory commit: %w", err)
	}
	return nil
}

func (d *Database) AppendAgentTurn(ctx context.Context, record repository.AgentTurnRecord) error {
	if d == nil || d.DB == nil {
		return fmt.Errorf("append agent turn: database is not initialized")
	}
	if strings.TrimSpace(record.IdentityKey) == "" || strings.TrimSpace(record.User) == "" || strings.TrimSpace(record.Assistant) == "" {
		return fmt.Errorf("append agent turn: identity and exchange content are required")
	}
	if record.ExpiresOnMillis <= record.CreatedOnMillis {
		return fmt.Errorf("append agent turn: expiry must be after creation")
	}
	for _, evidence := range record.Evidence {
		if strings.TrimSpace(evidence.ToolName) == "" || strings.TrimSpace(evidence.Content) == "" {
			return fmt.Errorf("append agent turn: evidence is invalid")
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("append agent turn: %w", err)
	}
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("append agent turn: %w", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM agent_memory WHERE expires_on <= ?1`, record.CreatedOnMillis); err != nil {
		return fmt.Errorf("append agent turn memory cleanup: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM agent_tool_memory WHERE expires_on <= ?1`, record.CreatedOnMillis); err != nil {
		return fmt.Errorf("append agent turn evidence cleanup: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM agent_memory_summary WHERE expires_on <= ?1`, record.CreatedOnMillis); err != nil {
		return fmt.Errorf("append agent turn summary cleanup: %w", err)
	}
	for _, message := range []struct{ role, content string }{{"user", record.User}, {"assistant", record.Assistant}} {
		if _, err = tx.ExecContext(ctx, `INSERT INTO agent_memory(identity_key, role, content, created_on, expires_on) VALUES (?1,?2,?3,?4,?5)`, record.IdentityKey, message.role, message.content, record.CreatedOnMillis, record.ExpiresOnMillis); err != nil {
			return fmt.Errorf("append agent turn %s: %w", message.role, err)
		}
	}
	for _, evidence := range record.Evidence {
		if _, err = tx.ExecContext(ctx, `INSERT INTO agent_tool_memory(identity_key, tool_name, content, created_on, expires_on) VALUES (?1,?2,?3,?4,?5)`, record.IdentityKey, evidence.ToolName, evidence.Content, record.CreatedOnMillis, record.ExpiresOnMillis); err != nil {
			return fmt.Errorf("append agent turn evidence: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("append agent turn commit: %w", err)
	}
	return nil
}

func (d *Database) LoadAgentToolEvidence(ctx context.Context, key string, nowMillis int64, limit int) ([]repository.AgentToolEvidence, error) {
	if d == nil || d.DB == nil {
		return nil, fmt.Errorf("load agent tool evidence: database is not initialized")
	}
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("load agent tool evidence: key is required")
	}
	if nowMillis < 0 {
		return nil, fmt.Errorf("load agent tool evidence: clock must not be negative")
	}
	if limit < 1 {
		return nil, fmt.Errorf("load agent tool evidence: limit must be positive")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load agent tool evidence: %w", err)
	}
	rows, err := d.DB.QueryContext(ctx, loadAgentToolEvidenceSQL, key, nowMillis, strconv.Itoa(limit))
	if err != nil {
		return nil, fmt.Errorf("load agent tool evidence: %w", err)
	}
	defer rows.Close()
	out := []repository.AgentToolEvidence{}
	for rows.Next() {
		var row repository.AgentToolEvidence
		if err := rows.Scan(&row.ToolName, &row.Content, &row.CreatedOnMillis); err != nil {
			return nil, fmt.Errorf("load agent tool evidence: %w", err)
		}
		if strings.TrimSpace(row.ToolName) == "" || strings.TrimSpace(row.Content) == "" || !json.Valid([]byte(row.Content)) || row.CreatedOnMillis < 0 {
			return nil, fmt.Errorf("load agent tool evidence: invalid stored evidence")
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load agent tool evidence: %w", err)
	}
	return out, nil
}

func (d *Database) AppendAgentToolEvidence(ctx context.Context, key, toolName, content string, createdOnMillis, expiresOnMillis int64) error {
	if d == nil || d.DB == nil {
		return fmt.Errorf("append agent tool evidence: database is not initialized")
	}
	if strings.TrimSpace(key) == "" || strings.TrimSpace(toolName) == "" || strings.TrimSpace(content) == "" {
		return fmt.Errorf("append agent tool evidence: key, tool name, and content are required")
	}
	if expiresOnMillis <= createdOnMillis {
		return fmt.Errorf("append agent tool evidence: expiry must be after creation")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("append agent tool evidence: %w", err)
	}
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("append agent tool evidence: %w", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM agent_tool_memory WHERE expires_on <= ?1`, createdOnMillis); err != nil {
		return fmt.Errorf("append agent tool evidence cleanup: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_tool_memory(identity_key, tool_name, content, created_on, expires_on) VALUES (?1,?2,?3,?4,?5)`, key, toolName, content, createdOnMillis, expiresOnMillis); err != nil {
		return fmt.Errorf("append agent tool evidence insert: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("append agent tool evidence commit: %w", err)
	}
	return nil
}

var _ repository.AgentMemoryRepository = (*Database)(nil)
var _ repository.AgentToolEvidenceRepository = (*Database)(nil)
var _ repository.AgentTurnRepository = (*Database)(nil)
