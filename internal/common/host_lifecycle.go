package common

import "context"

// HostLifecycleController is the narrow command-facing lifecycle request seam.
// A nil error means admitted or coalesced work, never callback completion.
type HostLifecycleController interface {
	RequestRestart(context.Context) error
	RequestShutdown(context.Context) error
}
