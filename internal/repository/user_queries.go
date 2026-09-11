package repository

import (
	"context"
	"database/sql"
	"errors"

	"zenbot/internal/model"
)

var ErrNotFound = errors.New("not found")

// ErrAmbiguousHistory means the exact nickname and trip match different public
// observation row sets. Private rows must not contribute to this classification.
var ErrAmbiguousHistory = errors.New("ambiguous public history")

// RegisteredUser is the persisted trip/nick pair rendered by !users.
type RegisteredUser struct {
	Trip string
	Name string
}

// LastOnlineRecord contains independent persisted public observations across
// stored rooms. Invalid fields mean the corresponding fact was not observed;
// neither a message nor a presence event proves current membership or a session.
type LastOnlineRecord struct {
	Found              bool
	LastMessage        sql.NullString
	LastMessageMillis  sql.NullInt64
	LastPresenceEvent  sql.NullString
	LastPresenceMillis sql.NullInt64
}

// UserQueryRepository is the deliberately narrow persistence seam for the
// Saturn users and nicks commands.
type UserQueryRepository interface {
	RegisteredUsers(context.Context) ([]RegisteredUser, error)
	// NicksByTrip returns exact historical names publicly observed with an exact
	// trip, latest first with a stable exact-name tie-break; it is not ownership.
	NicksByTrip(context.Context, string) ([]string, error)
	BasicUserData(context.Context, string, string) (string, error)
	// LastOnline accepts a raw nickname-or-trip selector: normalize the name
	// predicate once, preserve the exact trip, and reject ambiguous matches.
	LastOnline(context.Context, string) (LastOnlineRecord, error)
}

// RecentPresenceRepository resolves aliases observed during the recent-user
// notification window across current presence events and legacy message rows.
type RecentPresenceRepository interface {
	RecentPresenceNames(context.Context, string, string, int64, int) ([]string, error)
}

// LastSeenRepository keeps the legacy method name with the same public facts.
type LastSeenRepository interface {
	// LastSeen follows the same raw mixed-selector contract as LastOnline.
	LastSeen(context.Context, string) (LastOnlineRecord, error)
}

// IdentityRepository is the persistence seam for registration and message
// history. Implementations must make each mutating operation atomic.
type IdentityRepository interface {
	IsNameRegistered(context.Context, string) (bool, error)
	IsTripRegistered(context.Context, string) (bool, error)
	Register(context.Context, string, string, model.Role) error
	RegisterNameByTrip(context.Context, string, string) error
	RegisterTripByName(context.Context, string, string) error
	LastMessages(context.Context, string, string, int) ([]model.Message, error)
}
