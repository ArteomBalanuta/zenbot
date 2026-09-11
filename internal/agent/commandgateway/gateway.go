package commandgateway

import (
	"context"
	"encoding/json"

	"zenbot/internal/agent/api"
)

type OutcomeStatus string

const (
	OutcomeSucceeded OutcomeStatus = "SUCCEEDED"
	OutcomeRejected  OutcomeStatus = "REJECTED"
	OutcomeNotFound  OutcomeStatus = "NOT_FOUND"
	OutcomeAmbiguous OutcomeStatus = "AMBIGUOUS"
	OutcomeUnknown   OutcomeStatus = "UNKNOWN"
)

type DeliveryReceipt struct {
	Count int
}

// ActionReceipt records effects at the producing command's documented
// boundary, such as an outward write, a local mutation, or a delivered reply.
// It does not by itself prove whole-command or remote-server application. It is
// independent from room delivery because moderation requests can be silent.
type ActionReceipt struct {
	Count int
}

type Execution struct {
	Status OutcomeStatus
	// EffectsCommitted records producer-boundary effects even when Status is
	// rejected or unknown. It is not evidence that the whole requested command
	// succeeded or that a remote server applied an outbound request.
	EffectsCommitted bool
	Action           *ActionReceipt
	Messages         []string
	Delivery         *DeliveryReceipt
	Data             json.RawMessage
	// DataObserved is producer-owned validity, independent of effect status.
	DataObserved bool
}

type Gateway interface {
	Execute(context.Context, api.Context, string, string) (Execution, error)
}

// CanonicalAvailability is optional advisory visibility. Execute remains the
// authoritative source admission boundary and must recheck its pinned owner.
// Small compatibility gateways need only implement Gateway.
type CanonicalAvailability interface {
	CommandAvailable(canonical string) bool
}
