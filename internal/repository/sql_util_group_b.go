package repository

import "context"

type DeleteResult struct {
	TripNamesRows int64
	TripRows      int64
	NameRows      int64
}

type SaturnRegisteredUser struct {
	Name string
	Trip string
}

type SaturnLastMessage struct {
	Name      string
	Message   string
	CreatedOn int64
}

// SqlUtilGroupBRepository is an intentionally unwired compatibility seam.
// Implementations must require an already-authorized context for deletion.
type SqlUtilGroupBRepository interface {
	DeleteIdentity(context.Context, string, string) (DeleteResult, error)
	SaturnRegisteredUsers(context.Context) ([]SaturnRegisteredUser, error)
	SaturnLastMessages(context.Context, *string, string, int) ([]SaturnLastMessage, error)
}

// SaturnAuthorizedDeleteRepository is the narrow capability used by the
// runtime remove command. Implementations keep authorization details private.
type SaturnAuthorizedDeleteRepository interface {
	DeleteIdentityAuthorized(context.Context, string) (DeleteResult, error)
}
