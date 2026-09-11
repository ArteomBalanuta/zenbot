package common

import "context"

// LiveRoomMover resolves a normalized user selector on the host or managed
// replica serving the exact source channel before moving that canonical user.
// handled=false means no source is serving it.
type LiveRoomMover interface {
	MoveFromServingRoom(context.Context, string, string, Channel) (handled bool, err error)
}
