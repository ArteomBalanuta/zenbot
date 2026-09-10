package contract

// RoomUserSnapshot is the shared, immutable-at-return managed room user view.
type RoomUserSnapshot struct {
	Room  string
	Users []string
}
