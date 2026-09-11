package repository

import (
	"context"
	"zenbot/internal/model"
)

type DummyImpl struct{}

func (r *DummyImpl) LogMessage(trip, name, hash, message, channel string) (int64, error) {
	return -1, nil
}
func (r *DummyImpl) LogPresence(trip, name, hash, eventType, channel string) (int64, error) {
	return -1, nil
}
func (r *DummyImpl) LogCommand(context.Context, model.CommandAuditRecord) (int64, error) {
	return -1, nil
}
func (r *DummyImpl) Close() error { return nil }

// ShadowBanRepository persists source-compatible shadow-ban identity records.
// The narrow interface keeps privileged enforcement unavailable for repositories
// that do not implement the authoritative store.
type ShadowBanRepository interface {
	PersistShadowBan(context.Context, model.User, string) error
}

// ShadowBanManagementRepository is retained for compatibility with older
// command adapters. New production composition uses ShadowBanCommandRepository.
type ShadowBanManagementRepository interface {
	PersistShadowBanSelector(context.Context, string, string) error
	ListShadowBans(context.Context) ([]model.BanRecord, error)
	RemoveShadowBan(context.Context, string) error
}

// ShadowBanRecord is Saturn's local banned_users identity tuple. Hash is raw
// UTF-8 at this boundary; H2 encodes it for storage and decodes it on reads.
// Empty fields represent the source's nullable identity fields.
type ShadowBanRecord struct {
	Trip   string
	Name   string
	Hash   string
	Reason string
}

// ShadowBanCommandRepository is the typed persistence boundary used only by
// the public shadow-ban command family. It deliberately contains no agent
// policy or transport operation.
type ShadowBanCommandRepository interface {
	PersistShadowBanRecord(context.Context, ShadowBanRecord) error
	ListShadowBans(context.Context) ([]ShadowBanRecord, error)
	RemoveShadowBanBySourceTarget(context.Context, string) (int64, error)
	RemoveAllShadowBans(context.Context) (int64, error)
}

// ShadowBanReversalRepository removes source-compatible shadow-ban identity
// records for a captured authoritative target.
type ShadowBanReversalRepository interface {
	RemoveShadowBanBySourceTarget(context.Context, string) (int64, error)
}

type Repository interface {
	LogMessage(trip, name, hash, message, channel string) (int64, error)
	LogPresence(trip, name, hash, eventType, channel string) (int64, error)
	LogCommand(context.Context, model.CommandAuditRecord) (int64, error)
	Close() error
}

// AuditRepository is the typed persistence contract used by services and listeners.
type AuditRepository interface {
	MessageAudit(context.Context, model.MessageRecord) (int64, error)
	PresenceAudit(context.Context, model.PresenceRecord) (int64, error)
	CommandAudit(context.Context, model.CommandAuditRecord) (int64, error)
}

// AuthorizationRepository is the persisted trip/role boundary. Keeping it
// separate lets ZOMBIE engines use DummyImpl without opening a database.
type AuthorizationRepository interface {
	IsTripAuthorized(context.Context, string, model.Role, []string) (bool, error)
	GrantTrip(context.Context, string, model.Role) error
	GrantTrips(context.Context, []string, model.Role) error
	ResolveRole(context.Context, string) (model.Role, error)
}
