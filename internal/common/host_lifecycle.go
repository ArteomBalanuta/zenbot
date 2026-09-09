package common

import "context"

// HostLifecycleController is the narrow command-facing lifecycle request seam.
type HostLifecycleController interface {
	RequestRestart(context.Context) error
	RequestShutdown(context.Context) error
}
