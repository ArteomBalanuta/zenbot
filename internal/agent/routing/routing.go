// Package routing exposes shared request classification types without wiring
// the agent to tool, turn, memory, or provider dependencies.
package routing

import "zenbot/internal/agent/participation"

type RequestKind = participation.RequestKind
type ToolEvidence = participation.ToolEvidence
type Classifier = participation.Classifier

const (
	Unclassified = participation.Unclassified
	Talk         = participation.Talk
	ToolCall     = participation.ToolCall
)
