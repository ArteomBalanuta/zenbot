package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"zenbot/internal/model"
)

type reversalRepo struct {
	name  string
	calls int
	err   error
}

func (r *reversalRepo) LogMessage(string, string, string, string, string) (int64, error) {
	return 0, nil
}
func (r *reversalRepo) LogPresence(string, string, string, string, string) (int64, error) {
	return 0, nil
}
func (r *reversalRepo) LogCommand(context.Context, model.CommandAuditRecord) (int64, error) {
	return 0, nil
}
func (r *reversalRepo) Close() error { return nil }
func (r *reversalRepo) RemoveShadowBanBySourceTarget(_ context.Context, name string) (int64, error) {
	r.calls++
	r.name = name
	return 0, r.err
}

type basicRepository struct{}

func (basicRepository) LogMessage(string, string, string, string, string) (int64, error) {
	return 0, nil
}
func (basicRepository) LogPresence(string, string, string, string, string) (int64, error) {
	return 0, nil
}
func (basicRepository) LogCommand(context.Context, model.CommandAuditRecord) (int64, error) {
	return 0, nil
}
func (basicRepository) Close() error { return nil }

func TestNewModerationTargetCopiesListenerResolvedIdentity(t *testing.T) {
	user := model.User{Name: "author", Trip: "trip", Hash: "raw-hash"}
	target, err := NewModerationTarget(user)
	if err != nil {
		t.Fatal(err)
	}
	user.Name, user.Trip, user.Hash = "changed", "changed-trip", "changed-hash"
	if target.Name != "author" || target.Trip != "trip" || target.Hash != "raw-hash" {
		t.Fatalf("target = %#v", target)
	}
}

func TestNewModerationTargetRejectsBlankName(t *testing.T) {
	for _, name := range []string{"", " \t\n"} {
		if _, err := NewModerationTarget(model.User{Name: name, Hash: "raw-hash"}); err == nil {
			t.Fatalf("blank name %q accepted", name)
		}
	}
}

func TestUnmuteTargetSendsOnlySourceWirePayload(t *testing.T) {
	e := &EngineImpl{OutMessageQueue: make(chan string, 1)}
	target, err := NewModerationTarget(model.User{Name: "author", Trip: "trip", Hash: "hash-a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.UnmuteTarget(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if got := <-e.OutMessageQueue; got != `{"cmd":"unmute","hash":"hash-a"}` {
		t.Fatalf("unmute payload = %q", got)
	}
	if len(e.OutMessageQueue) != 0 {
		t.Fatalf("unmute emitted unexpected extra output: %d", len(e.OutMessageQueue))
	}
}

func TestUnmuteTargetFailsClosedForBlankHashAndCancelledContext(t *testing.T) {
	e := &EngineImpl{OutMessageQueue: make(chan string, 1)}
	if err := e.UnmuteTarget(context.Background(), ModerationTarget{Name: "author"}); err == nil {
		t.Fatal("blank hash accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.UnmuteTarget(ctx, ModerationTarget{Name: "author", Hash: "hash-a"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context error = %v", err)
	}
	if len(e.OutMessageQueue) != 0 {
		t.Fatalf("failed unmute emitted output: %d", len(e.OutMessageQueue))
	}
}

func TestUnmuteTargetHonorsBlockedOutboundDeadline(t *testing.T) {
	e := &EngineImpl{OutMessageQueue: make(chan string)}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := e.UnmuteTarget(ctx, ModerationTarget{Name: "author", Hash: "hash-a"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked unmute error = %v", err)
	}
}

func TestUnshadowBanTargetDelegatesOnlyAuthoritativeNameWithoutOutput(t *testing.T) {
	repo := &reversalRepo{}
	e := &EngineImpl{Repository: repo, OutMessageQueue: make(chan string, 1)}
	target, err := NewModerationTarget(model.User{Name: "author", Trip: "trip", Hash: "raw-hash"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.UnshadowBanTarget(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if repo.calls != 1 || repo.name != "author" {
		t.Fatalf("repository calls=%d name=%q", repo.calls, repo.name)
	}
	if len(e.OutMessageQueue) != 0 {
		t.Fatalf("unshadowban emitted output: %d", len(e.OutMessageQueue))
	}
}

func TestUnshadowBanTargetFailsClosedWithoutCapabilityBlankNameOrCancelledContext(t *testing.T) {
	for _, tc := range []struct {
		name   string
		engine *EngineImpl
		ctx    context.Context
		target ModerationTarget
	}{
		{"unavailable repository", &EngineImpl{Repository: basicRepository{}, OutMessageQueue: make(chan string, 1)}, context.Background(), ModerationTarget{Name: "author"}},
		{"blank name", &EngineImpl{Repository: &reversalRepo{}, OutMessageQueue: make(chan string, 1)}, context.Background(), ModerationTarget{}},
		{"cancelled context", &EngineImpl{Repository: &reversalRepo{}, OutMessageQueue: make(chan string, 1)}, cancelledContext(t), ModerationTarget{Name: "author"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.engine.UnshadowBanTarget(tc.ctx, tc.target); err == nil {
				t.Fatal("operation unexpectedly succeeded")
			}
			if len(tc.engine.OutMessageQueue) != 0 {
				t.Fatalf("failed unshadowban emitted output: %d", len(tc.engine.OutMessageQueue))
			}
		})
	}
}

func cancelledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
