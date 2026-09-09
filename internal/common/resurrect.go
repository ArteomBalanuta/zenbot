package common

import "context"

// LiveRoomMover moves a target only when a host or managed replica already
// serves the exact source channel. handled=false means no source is serving it.
type LiveRoomMover interface {
	MoveFromServingRoom(context.Context, string, NickTarget, Channel) (handled bool, err error)
}
