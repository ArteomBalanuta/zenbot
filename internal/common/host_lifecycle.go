package common

import "context"

// HostLifecycleController is the narrow command-facing lifecycle request seam.
// A nil error means admitted or coalesced work, never callback completion.
type HostLifecycleController interface {
	RequestRestart(context.Context) error
	RequestShutdown(context.Context) error
}

// LifecycleResult is source completion evidence, separate from admission.
// Err is for trusted callers only and must never be serialized for a model.
type LifecycleResult struct {
	Kind string
	Err  error
}

// LifecycleOperation exposes read-only source evidence. Result returns a
// nonblocking snapshot; completion is established only when finished is true.
// Dispatchers must release their lease before any wait on Done.
type LifecycleOperation interface {
	Done() <-chan struct{}
	Result() (result LifecycleResult, finished bool)
}

type LifecycleAdmission struct {
	Operation LifecycleOperation
	Coalesced bool
}

// HostLifecycleRequests extends the error-only compatibility contract with
// structured request admission. It does not promise completed host retirement.
// A non-nil error guarantees that this invocation neither inserted a request
// nor coalesced with one. Once admitted, return the admission with a nil error,
// including when cancellation arrives after insertion.
type HostLifecycleRequests interface {
	HostLifecycleController
	RequestRestartResult(context.Context) (LifecycleAdmission, error)
	RequestShutdownResult(context.Context) (LifecycleAdmission, error)
}

// LifecycleRequestRejectedError preserves explicit source evidence of
// non-admission. It may also mark cancellation checked before calling the source.
// It must never wrap an uncertain error from an error-only compatibility source.
type LifecycleRequestRejectedError struct{ Err error }

func (e *LifecycleRequestRejectedError) Error() string { return e.Err.Error() }
func (e *LifecycleRequestRejectedError) Unwrap() error { return e.Err }
