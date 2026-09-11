package commandgateway

import (
	"context"

	"zenbot/internal/agent/api"
)

type OutcomeStatus string

const (
	OutcomeSucceeded OutcomeStatus = "SUCCEEDED"
	OutcomeRejected  OutcomeStatus = "REJECTED"
	OutcomeNotFound  OutcomeStatus = "NOT_FOUND"
	OutcomeUnknown   OutcomeStatus = "UNKNOWN"
)

type DeliveryReceipt struct {
	Count int
}

// ActionReceipt verifies that the command caused one or more outward effects.
// It is independent from room delivery because moderation actions can be
// intentionally silent.
type ActionReceipt struct {
	Count int
}

type Execution struct {
	Status OutcomeStatus
	// EffectsCommitted records observed effects even when Status is rejected
	// or unknown. It is not evidence that the whole requested command succeeded.
	EffectsCommitted bool
	Action           *ActionReceipt
	Messages         []string
	Delivery         *DeliveryReceipt
}

type Gateway interface {
	Execute(context.Context, api.Context, string, string) (Execution, error)
}
