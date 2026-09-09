package listener

import (
	"context"
	"testing"

	"zenbot/internal/core"
	"zenbot/internal/model"
)

type leftPresenceRepository struct {
	messages int
	presence []model.PresenceRecord
}

func (r *leftPresenceRepository) LogMessage(string, string, string, string, string) (int64, error) {
	r.messages++
	return 0, nil
}
func (r *leftPresenceRepository) LogPresence(trip, name, hash, event, channel string) (int64, error) {
	r.presence = append(r.presence, model.PresenceRecord{Trip: trip, Name: name, Hash: hash, EventType: event, Channel: channel})
	return int64(len(r.presence)), nil
}
func (*leftPresenceRepository) LogCommand(context.Context, model.CommandAuditRecord) (int64, error) {
	return 0, nil
}
func (*leftPresenceRepository) Close() error { return nil }

func TestUserLeftListenerIgnoresUnknownUserWithoutPanicking(t *testing.T) {
	repo := &leftPresenceRepository{}
	engine := &core.EngineImpl{Channel: "programming", ActiveUsers: map[*model.User]struct{}{}, Repository: repo}

	NewUserLeftListener(engine).Notify(`{"nick":"ghost"}`)

	if len(repo.presence) != 0 || repo.messages != 0 {
		t.Fatalf("unexpected audit: presence=%v messages=%d", repo.presence, repo.messages)
	}
}

func TestUserLeftListenerAuditsPresenceAndRemovesKnownUser(t *testing.T) {
	repo := &leftPresenceRepository{}
	engine := &core.EngineImpl{Channel: "programming", ActiveUsers: map[*model.User]struct{}{}, Repository: repo}
	engine.AddActiveUser(&model.User{Name: "alice", Trip: "trip", Hash: "hash"})

	NewUserLeftListener(engine).Notify(`{"nick":"alice"}`)

	if engine.GetActiveUserByName("alice") != nil || len(repo.presence) != 1 || repo.presence[0].EventType != "left" || repo.messages != 0 {
		t.Fatalf("active=%v presence=%v messages=%d", engine.GetActiveUserByName("alice"), repo.presence, repo.messages)
	}
}
