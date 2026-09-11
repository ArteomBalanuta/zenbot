package common

import (
	"context"
	"sync/atomic"
)

type mutationRecorderKey struct{}

// MutationRecorder retains only known committed operation counts. Each command
// invocation owns its recorder; shared services never retain it.
type MutationRecorder struct{ count atomic.Int64 }

// WithMutationRecorder starts an isolated invocation, even when ctx already
// carries a recorder. Count returns a detached snapshot on any exit.
func WithMutationRecorder(ctx context.Context) (context.Context, *MutationRecorder) {
	r := &MutationRecorder{}
	return context.WithValue(ctx, mutationRecorderKey{}, r), r
}

func (r *MutationRecorder) Count() int { return int(r.count.Load()) }

// RecordCommittedMutation must follow the authoritative successful write or
// commit. Cancellation does not erase an already known fact. Without a recorder
// this is a no-op; no payload or identity is retained.
func RecordCommittedMutation(ctx context.Context) {
	if ctx == nil {
		return
	}
	if r, ok := ctx.Value(mutationRecorderKey{}).(*MutationRecorder); ok {
		r.count.Add(1)
	}
}
