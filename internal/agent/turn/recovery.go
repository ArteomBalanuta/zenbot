package turn

import "zenbot/internal/agent/tool/contract"

type RecoveryDecision string

const (
	RecoveryRetryModel RecoveryDecision = "RETRY_MODEL"
	RecoveryDegrade    RecoveryDecision = "DEGRADE"
	RecoveryFinalize   RecoveryDecision = "FINALIZE"
	RecoveryFail       RecoveryDecision = "FAIL"
)

type RecoveryInput struct {
	ErrorCode           string
	Effect              contract.Effect
	Idempotent          bool
	ToolRoundsRemaining int
	HasObservations     bool
}

type RecoveryPolicy struct{}

func (RecoveryPolicy) Decide(input RecoveryInput) RecoveryDecision {
	if input.ErrorCode == "ACTION_OUTCOME_UNKNOWN" && input.Effect == contract.Action {
		return RecoveryFinalize
	}
	if input.ToolRoundsRemaining > 0 {
		switch input.ErrorCode {
		case "INVALID_ARGUMENTS", "UNKNOWN_TOOL", "TOOL_NOT_ALLOWED", "TOOL_EXECUTION_FAILED", "TOOL_TIMEOUT", "INVALID_TOOL_RESULT":
			return RecoveryRetryModel
		}
	}
	switch input.ErrorCode {
	case "TOOL_DISABLED", "TOOL_CALL_LIMIT_REACHED", "DUPLICATE_TOOL_CALL", "MISSING_PREREQUISITE":
		return RecoveryDegrade
	}
	if input.HasObservations {
		return RecoveryFinalize
	}
	return RecoveryFail
}
