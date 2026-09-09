package repository

import (
	"context"
	"encoding/json"
	"strings"
)

// PublicRoomMessage is untrusted public-room evidence for agent prompt context.
type PublicRoomMessage struct {
	Name, Trip, Hash, Message, Channel string
	CreatedOnMillis                    int64
}

// AgentConversationRepository reads bounded public room context.
type AgentConversationRepository interface {
	RecentPublicRoomMessages(context.Context, string, int) ([]PublicRoomMessage, error)
}

type AgentUserMessageHistoryRepository interface {
	RecentPublicRoomMessagesForNick(context.Context, string, string, int) ([]PublicRoomMessage, error)
}

type AgentNamedQueryRepository interface {
	ExecuteAgentQuery(context.Context, string, json.RawMessage, string, string) (json.RawMessage, error)
}

type AgentSchemaRepository interface {
	DescribeAgentSchema(context.Context) (AgentDatabaseSchema, error)
}

type AgentSQLRepository interface {
	ExecuteAgentSQL(context.Context, string, int, int, int, int) (json.RawMessage, error)
}

// AgentDatabaseSchema is immutable-by-convention metadata exposed to the agent.
type AgentDatabaseSchema struct {
	Tables []AgentDatabaseTable `json:"tables"`
}
type AgentDatabaseTable struct {
	Name        string                    `json:"name"`
	Columns     []AgentDatabaseColumn     `json:"columns"`
	Indexes     []AgentDatabaseIndex      `json:"indexes"`
	ForeignKeys []AgentDatabaseForeignKey `json:"foreignKeys"`
}
type AgentDatabaseColumn struct {
	Ordinal    int    `json:"ordinal"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primaryKey"`
}
type AgentDatabaseIndex struct {
	Name    string   `json:"name"`
	Unique  bool     `json:"unique"`
	Columns []string `json:"columns"`
}
type AgentDatabaseForeignKey struct {
	ID              int    `json:"id"`
	Sequence        int    `json:"sequence"`
	ReferencedTable string `json:"referencedTable"`
	FromColumn      string `json:"fromColumn"`
	ToColumn        string `json:"toColumn"`
	OnUpdate        string `json:"onUpdate"`
	OnDelete        string `json:"onDelete"`
	Match           string `json:"match"`
}

func (s AgentDatabaseSchema) TableNames() []string {
	out := make([]string, 0, len(s.Tables))
	for _, t := range s.Tables {
		out = append(out, t.Name)
	}
	return out
}
func (s AgentDatabaseSchema) FindTable(name string) (AgentDatabaseTable, bool) {
	for _, t := range s.Tables {
		if strings.EqualFold(t.Name, name) {
			return t, true
		}
	}
	return AgentDatabaseTable{}, false
}
