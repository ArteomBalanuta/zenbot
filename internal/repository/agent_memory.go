package repository

import "context"

// AgentMemoryMessage is one bounded durable conversation message.
type AgentMemoryMessage struct{ Role, Content string }

// AgentMemoryRepository owns exact-key durable turn storage.
type AgentMemoryRepository interface {
	LoadAgentMemory(context.Context, string, int64, int) ([]AgentMemoryMessage, error)
	AppendAgentMemory(context.Context, string, string, string, int64, int64) error
}

// AgentToolEvidence is one validated, provenance-tagged durable tool result.
type AgentToolEvidence struct {
	ToolName        string
	Content         string
	CreatedOnMillis int64
}

// AgentToolEvidenceRepository owns exact-key durable successful tool evidence.
type AgentToolEvidenceRepository interface {
	LoadAgentToolEvidence(context.Context, string, int64, int) ([]AgentToolEvidence, error)
	AppendAgentToolEvidence(context.Context, string, string, string, int64, int64) error
}
