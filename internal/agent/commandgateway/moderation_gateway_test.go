package commandgateway

import (
	"context"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/core"
)

type moderationOperatorStub struct{ action, name, hash string }

func (s *moderationOperatorStub) EnableCaptcha(context.Context) error {
	s.action = "captcha"
	return nil
}
func (s *moderationOperatorStub) MutePrincipal(_ context.Context, name string) error {
	s.action, s.name = "mute", name
	return nil
}
func (s *moderationOperatorStub) UnmuteTarget(_ context.Context, target core.ModerationTarget) error {
	s.action, s.hash = "unmute", target.Hash
	return nil
}
func (s *moderationOperatorStub) KickPrincipal(_ context.Context, name string) error {
	s.action, s.name = "kick", name
	return nil
}
func (s *moderationOperatorStub) ShadowBan(_ context.Context, name string) error {
	s.action, s.name = "shadowban", name
	return nil
}
func (s *moderationOperatorStub) UnshadowBanTarget(_ context.Context, target core.ModerationTarget) error {
	s.action, s.name = "unshadowban", target.Name
	return nil
}

func TestModerationGatewayMapsCapturedTargetWithoutModelArguments(t *testing.T) {
	op := &moderationOperatorStub{}
	gateway := NewModerationGateway(op)
	ctx, err := api.NewContextWithCapabilities("room", "bot", "creator", "", false, []string{}, []api.Capability{api.ModerationCommands})
	if err != nil {
		t.Fatal(err)
	}
	if err := gateway.Execute(context.Background(), api.MODERATION, ctx, core.ModerationTarget{Name: "alice", Trip: "trip", Hash: "raw-hash"}, ActionUnmute); err != nil {
		t.Fatal(err)
	}
	if op.action != "unmute" || op.hash != "raw-hash" {
		t.Fatalf("operation = %+v", op)
	}
}

func TestModerationGatewayRejectsWrongModeWithoutOperation(t *testing.T) {
	op := &moderationOperatorStub{}
	gateway := NewModerationGateway(op)
	ctx, _ := api.NewContextWithCapabilities("room", "bot", "creator", "", false, []string{}, []api.Capability{api.ModerationCommands})
	if err := gateway.Execute(context.Background(), api.DIRECT, ctx, core.ModerationTarget{Name: "alice"}, ActionMute); err == nil {
		t.Fatal("wrong mode accepted")
	}
	if op.action != "" {
		t.Fatalf("operation = %q", op.action)
	}
}
