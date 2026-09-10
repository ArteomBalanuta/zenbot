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

type Execution struct {
	Status           OutcomeStatus
	EffectsCommitted bool
	Messages         []string
	Delivery         *DeliveryReceipt
}

type Gateway interface {
	Execute(context.Context, api.Context, string, string) (Execution, error)
}
