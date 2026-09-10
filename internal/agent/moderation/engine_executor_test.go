package moderation

import (
	"context"
	"errors"
	"testing"
)

type moderationEngineStub struct {
	captcha, shadow, ban int
	err                  error
}

func (s *moderationEngineStub) EnableCaptcha(context.Context) error     { s.captcha++; return s.err }
func (s *moderationEngineStub) ShadowBan(context.Context, string) error { s.shadow++; return s.err }

func TestEngineActionExecutorUsesOneAuthoritativeOperation(t *testing.T) {
	e := &moderationEngineStub{}
	x := NewEngineActionExecutor(e)
	if err := x.Execute(context.Background(), Decision{Action: Captcha}); err != nil {
		t.Fatal(err)
	}
	if e.captcha != 1 || e.shadow != 0 || e.ban != 0 {
		t.Fatalf("%+v", e)
	}
	if err := x.Execute(context.Background(), Decision{Action: ShadowBan, Principal: "raider"}); err != nil {
		t.Fatal(err)
	}
	if e.captcha != 1 || e.shadow != 1 || e.ban != 0 {
		t.Fatalf("%+v", e)
	}
}
func TestEngineActionExecutorRejectsInvalidCancelledAndFailureWithoutRetry(t *testing.T) {
	e := &moderationEngineStub{}
	x := NewEngineActionExecutor(e)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := x.Execute(ctx, Decision{Action: Captcha}); err == nil || e.captcha != 0 {
		t.Fatalf("err=%v state=%+v", err, e)
	}
	if err := x.Execute(context.Background(), Decision{Action: ShadowBan}); err == nil || e.shadow != 0 {
		t.Fatalf("err=%v state=%+v", err, e)
	}
	e.err = errors.New("broken")
	if err := x.Execute(context.Background(), Decision{Action: Captcha}); err == nil || e.captcha != 1 {
		t.Fatalf("err=%v state=%+v", err, e)
	}
}
