package core

import (
	"context"
	"testing"
	"time"

	"zenbot/internal/model"
)

type shadowRepo struct {
	user  model.User
	calls int
}

func (r *shadowRepo) LogMessage(string, string, string, string, string) (int64, error) { return 0, nil }
func (r *shadowRepo) LogPresence(string, string, string, string, string) (int64, error) {
	return 0, nil
}
func (r *shadowRepo) LogCommand(context.Context, model.CommandAuditRecord) (int64, error) {
	return 0, nil
}
func (r *shadowRepo) Close() error { return nil }
func (r *shadowRepo) PersistShadowBan(_ context.Context, u model.User, _ string) error {
	r.calls++
	r.user = u
	return nil
}
func TestAuthoritativeCaptchaAndShadowBan(t *testing.T) {
	r := &shadowRepo{}
	e := &EngineImpl{OutMessageQueue: make(chan string, 1), ActiveUsers: map[*model.User]struct{}{}, Repository: r}
	u := &model.User{Name: "raider", Trip: "trip", Hash: "hash"}
	e.AddActiveUser(u)
	if err := e.EnableCaptcha(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := <-e.OutMessageQueue; got != "{ \"cmd\": \"enablecaptcha\"}" {
		t.Fatalf("%q", got)
	}
	if err := e.ShadowBan(context.Background(), "raider"); err != nil {
		t.Fatal(err)
	}
	if r.calls != 1 || r.user.Hash != "hash" {
		t.Fatalf("%+v", r)
	}
}

func TestEnableCaptchaHonorsExecutionDeadlineWhenOutboundQueueIsBlocked(t *testing.T) {
	e := &EngineImpl{OutMessageQueue: make(chan string)}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := e.EnableCaptcha(ctx); err == nil {
		t.Fatal("expected blocked authoritative captcha operation to honor its deadline")
	}
}

func TestAuthoritativeMessageModerationUsesSafeFixedProtocol(t *testing.T) {
	e := &EngineImpl{OutMessageQueue: make(chan string, 3), ActiveUsers: map[*model.User]struct{}{}}
	e.AddActiveUser(&model.User{Name: "Raider", Trip: "trip", Hash: "hash"})
	if err := e.WarnFlood(context.Background(), "raider"); err != nil {
		t.Fatal(err)
	}
	if got := <-e.OutMessageQueue; got != `{"cmd":"chat","text":"@Raider Please stop flooding."}` {
		t.Fatalf("warning=%q", got)
	}
	if err := e.MutePrincipal(context.Background(), "raider"); err != nil {
		t.Fatal(err)
	}
	if got := <-e.OutMessageQueue; got != `{"cmd":"mute","nick":"Raider"}` {
		t.Fatalf("mute=%q", got)
	}
	if err := e.KickPrincipal(context.Background(), "raider"); err != nil {
		t.Fatal(err)
	}
	if got := <-e.OutMessageQueue; got != `{"cmd":"kick","nick":"Raider"}` {
		t.Fatalf("kick=%q", got)
	}
}

func TestAuthoritativeMessageModerationRejectsInactivePrincipal(t *testing.T) {
	e := &EngineImpl{OutMessageQueue: make(chan string, 3), ActiveUsers: map[*model.User]struct{}{}}
	for _, operation := range []struct {
		name string
		run  func(context.Context, string) error
	}{
		{"warning", e.WarnFlood},
		{"mute", e.MutePrincipal},
		{"kick", e.KickPrincipal},
	} {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.run(context.Background(), "gone"); err == nil {
				t.Fatal("inactive principal was accepted")
			}
		})
	}
	if len(e.OutMessageQueue) != 0 {
		t.Fatalf("inactive principal sent moderation payloads: %d", len(e.OutMessageQueue))
	}
}
