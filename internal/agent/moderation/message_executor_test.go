package moderation

import (
	"context"
	"errors"
	"testing"
)

type messageModeratorStub struct {
	warn, mute, kick, shadow int
	err                      error
}

func (s *messageModeratorStub) WarnFlood(context.Context, string) error     { s.warn++; return s.err }
func (s *messageModeratorStub) MutePrincipal(context.Context, string) error { s.mute++; return s.err }
func (s *messageModeratorStub) KickPrincipal(context.Context, string) error { s.kick++; return s.err }
func (s *messageModeratorStub) ShadowBan(context.Context, string) error     { s.shadow++; return s.err }

func TestMessageActionExecutorExecutesExactlyOneAuthoritativeAction(t *testing.T) {
	s := &messageModeratorStub{}
	x := NewMessageActionExecutor(s)
	for _, d := range []Decision{{Action: Warn, Principal: "a"}, {Action: Mute, Principal: "a"}, {Action: Kick, Principal: "a"}, {Action: ShadowBan, Principal: "a"}} {
		if err := x.Execute(context.Background(), d); err != nil {
			t.Fatal(err)
		}
	}
	if s.warn != 1 || s.mute != 1 || s.kick != 1 || s.shadow != 1 {
		t.Fatalf("calls=%+v", s)
	}
}
func TestMessageActionExecutorRejectsCancelledInvalidAndFailureWithoutRetry(t *testing.T) {
	s := &messageModeratorStub{}
	x := NewMessageActionExecutor(s)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := x.Execute(ctx, Decision{Action: Warn, Principal: "a"}); err == nil || s.warn != 0 {
		t.Fatalf("cancel err=%v calls=%+v", err, s)
	}
	if err := x.Execute(context.Background(), Decision{Action: Captcha}); err == nil || s.warn+s.mute+s.kick+s.shadow != 0 {
		t.Fatalf("invalid err=%v calls=%+v", err, s)
	}
	s.err = errors.New("nope")
	if err := x.Execute(context.Background(), Decision{Action: Kick, Principal: "a"}); err == nil || s.kick != 1 {
		t.Fatalf("failure err=%v calls=%+v", err, s)
	}
}
