package common

import "context"

type AutoMoveSnapshot struct {
	Enabled     bool
	Sources     []string
	Destination string
}

type AutoMoveController interface {
	AutoMoveSnapshot() AutoMoveSnapshot
	ConfigureAutoMove(source, destination string) (AutoMoveSnapshot, error)
	EnableAutoMove(context.Context) (AutoMoveSnapshot, error)
	DisableAutoMove(context.Context) (AutoMoveSnapshot, error)
}

// AutoMoveJoinPolicy reports whether an exact replica channel is currently eligible.
type AutoMoveJoinPolicy interface {
	EligibleReplica(channel string) (destination string, ok bool)
}
