package core

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/model"
	"zenbot/internal/repository"
)

func TestSharedDispatchAuditPassesCancellationToWhisperInfoListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	listener := &contextualChatNotifier{}
	engine := &EngineImpl{UserInfoListener: listener}
	engine.DispatchMessageContext(ctx, `{"cmd":"info","type":"whisper","from":"alice","text":"alice whispered: !restart"}`)
	if listener.notifyCalls != 0 || listener.notifyContextCalls != 1 || listener.ctx != ctx {
		t.Fatalf("whisper command lost lifecycle cancellation: legacy=%d contextual=%d context=%v", listener.notifyCalls, listener.notifyContextCalls, listener.ctx)
	}
}

type legacyDispatchAuditRepository struct {
	repository.DummyImpl
	bodies []string
}

func (r *legacyDispatchAuditRepository) LogMessage(_, _, _, body, _ string) (int64, error) {
	r.bodies = append(r.bodies, body)
	return 1, nil
}

func TestSharedDispatchAuditPublicRepositoryFallbackPreservesPrivateBoundary(t *testing.T) {
	repo := &legacyDispatchAuditRepository{}
	engine := &EngineImpl{Repository: repo}
	_, err := engine.LogMessageRecord(context.Background(), model.MessageRecord{Message: "public", Visibility: "PUBLIC"})
	if err != nil || len(repo.bodies) != 1 || repo.bodies[0] != "public" {
		t.Fatalf("legacy public audit stopped: err=%v bodies=%v", err, repo.bodies)
	}
	for _, visibility := range []string{"WHISPER", ""} {
		_, err = engine.LogMessageRecord(context.Background(), model.MessageRecord{Message: "private", Visibility: visibility})
		if err == nil || len(repo.bodies) != 1 {
			t.Fatalf("private/unknown audit leaked: visibility=%q err=%v bodies=%v", visibility, err, repo.bodies)
		}
	}
}

type failingDispatchAuditRepository struct {
	legacyDispatchAuditRepository
	failure error
}

func (r *failingDispatchAuditRepository) MessageAudit(context.Context, model.MessageRecord) (int64, error) {
	return 0, r.failure
}
func (*failingDispatchAuditRepository) PresenceAudit(context.Context, model.PresenceRecord) (int64, error) {
	return 0, nil
}
func (*failingDispatchAuditRepository) CommandAudit(context.Context, model.CommandAuditRecord) (int64, error) {
	return 0, nil
}

func TestSharedDispatchAuditTypedRepositoryFailureDoesNotFallBack(t *testing.T) {
	repo := &failingDispatchAuditRepository{failure: errors.New("typed store failed")}
	engine := &EngineImpl{Repository: repo}
	_, err := engine.LogMessageRecord(context.Background(), model.MessageRecord{Message: "public", Visibility: "PUBLIC"})
	if !errors.Is(err, repo.failure) || len(repo.bodies) != 0 {
		t.Fatalf("typed failure lost: err=%v legacy=%v", err, repo.bodies)
	}
}

func TestSharedDispatchAuditPrecancelStopsRepositoryWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	legacy := &legacyDispatchAuditRepository{}
	typed := &failingDispatchAuditRepository{failure: errors.New("typed store called")}
	for _, repo := range []repository.Repository{legacy, typed} {
		engine := &EngineImpl{Repository: repo}
		_, err := engine.LogMessageRecord(ctx, model.MessageRecord{Message: "public", Visibility: "PUBLIC"})
		if !errors.Is(err, context.Canceled) || len(legacy.bodies) != 0 || len(typed.bodies) != 0 {
			t.Fatalf("canceled audit ran: err=%v legacy=%v typed=%v", err, legacy.bodies, typed.bodies)
		}
	}
}
