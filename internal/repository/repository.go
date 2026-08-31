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
	ResolveRole(context.Context, string) (model.Role, error)
}
