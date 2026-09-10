package turn

import (
	"context"

	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool/contract"
)

// CompletionDecision is the semantic outcome for a candidate final answer.
type CompletionDecision string

type CompletionReplyMode string

const (
	CompletionFinal    CompletionDecision  = "FINAL"
	CompletionContinue CompletionDecision  = "CONTINUE"
	CompletionSend     CompletionReplyMode = "SEND"
	CompletionSuppress CompletionReplyMode = "SUPPRESS"
)

// ToolCapability is the caller-visible semantic summary available to the
// completion evaluator. Execution remains owned by the registry.
type ToolCapability struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ToolCallEvidence binds a result to the exact model-selected arguments that
// produced it. Completion evaluators use it as evidence; they cannot execute it.
type ToolCallEvidence struct {
	CallID     string
	Tool       string
	Arguments  string
	Effect     contract.Effect
	ResultMode contract.ResultMode
}

// CompletionCandidate contains the evidence needed to determine whether the
// newest request is actually complete, independently of response syntax.
type CompletionCandidate struct {
	Request      string
	Answer       string
	CanContinue  bool
	Conversation []llm.LlmMessage
	Tools        []ToolCapability
	Calls        []ToolCallEvidence
	Results      []contract.Result
}

// CompletionAssessment either approves a final answer or supplies a
// self-contained instruction for the next model/tool cycle.
type CompletionAssessment struct {
	Decision  CompletionDecision
	Feedback  string
	ReplyMode CompletionReplyMode
}

// CompletionGate semantically evaluates every candidate final response. It
// must not execute tools or infer decisions through keyword heuristics.
type CompletionGate interface {
	Evaluate(context.Context, CompletionCandidate) (CompletionAssessment, error)
}
