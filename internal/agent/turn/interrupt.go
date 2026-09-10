package turn

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
)

type InterruptOutcome string

const (
	InterruptAllow InterruptOutcome = "ALLOW"
	InterruptDeny  InterruptOutcome = "DENY"
	InterruptPause InterruptOutcome = "PAUSE"
)

type ActionRequest struct {
	Call       execution.Call
	Descriptor contract.Descriptor
	Context    api.Context
}

type InterruptDecision struct {
	Outcome     InterruptOutcome
	Reason      string
	ResumeToken string
}

func NewInterruptDecision(outcome InterruptOutcome, reason, resumeToken string) (InterruptDecision, error) {
	decision := InterruptDecision{Outcome: outcome, Reason: strings.TrimSpace(reason), ResumeToken: strings.TrimSpace(resumeToken)}
	switch outcome {
	case InterruptAllow:
		return decision, nil
	case InterruptDeny:
		if decision.Reason == "" {
			return InterruptDecision{}, fmt.Errorf("interrupt denial requires a reason")
		}
	case InterruptPause:
		if decision.Reason == "" || decision.ResumeToken == "" {
			return InterruptDecision{}, fmt.Errorf("interrupt pause requires a reason and resume token")
		}
	default:
		return InterruptDecision{}, fmt.Errorf("unknown interrupt outcome: %s", outcome)
	}
	return decision, nil
}

type InterruptHook interface {
	Review(context.Context, ActionRequest) (InterruptDecision, error)
}

type AllowAllInterruptHook struct{}

func (AllowAllInterruptHook) Review(context.Context, ActionRequest) (InterruptDecision, error) {
	return InterruptDecision{Outcome: InterruptAllow}, nil
}

type PendingAction struct {
	Request     ActionRequest
	ResumeToken string
	Reason      string
}

type PausedError struct{ Pending PendingAction }

func (e *PausedError) Error() string { return "agent turn paused: " + e.Pending.Reason }
