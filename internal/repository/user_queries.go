package repository

import (
	"context"
	"database/sql"
	"errors"

	"zenbot/internal/model"
)

var ErrNotFound = errors.New("not found")

// RegisteredUser is the persisted trip/nick pair rendered by !users.
type RegisteredUser struct {
	Trip string
	Name string
}

// LastOnlineRecord is the raw message-history state needed to render Saturn's
// last-online reply. Invalid fields mean the corresponding query found no row.
type LastOnlineRecord struct {
	Found          bool
	LastMessage    sql.NullString
	LastSeenMillis sql.NullInt64
	JoinedMillis   sql.NullInt64
}

// UserQueryRepository is the deliberately narrow persistence seam for the
// Saturn users and nicks commands.
type UserQueryRepository interface {
	RegisteredUsers(context.Context) ([]RegisteredUser, error)
	NicksByTrip(context.Context, string) ([]string, error)
	BasicUserData(context.Context, string, string) (string, error)
	LastOnline(context.Context, string) (LastOnlineRecord, error)
}

// RecentPresenceRepository resolves aliases observed during the recent-user
// notification window across current presence events and legacy message rows.
type RecentPresenceRepository interface {
	RecentPresenceNames(context.Context, string, string, int64, int) ([]string, error)
}

// LastSeen preserves observations rendered by Saturn's last-online command.
type LastSeen struct {
	Message  string
	SeenAt   *int64
	JoinedAt *int64
}

type LastSeenRepository interface {
	LastSeen(context.Context, string) (LastSeen, error)
}

// IdentityRepository is the persistence seam for registration and message
// history. Implementations must make each mutating operation atomic.
type IdentityRepository interface {
	IsNameRegistered(string) (bool, error)
	IsTripRegistered(string) (bool, error)
	Register(string, string, model.Role) error
	RegisterNameByTrip(string, string) error
	RegisterTripByName(string, string) error
	LastMessages(string, string, int) ([]model.Message, error)
}
