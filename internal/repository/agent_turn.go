package repository

import "context"

type AgentTurnEvidence struct {
	ToolName string
	Content  string
}

// AgentTurnRecord is the atomic durable unit for one completed request.
type AgentTurnRecord struct {
	IdentityKey     string
	User            string
	Assistant       string
	Evidence        []AgentTurnEvidence
	CreatedOnMillis int64
	ExpiresOnMillis int64
}

type AgentTurnRepository interface {
	AppendAgentTurn(context.Context, AgentTurnRecord) error
}
