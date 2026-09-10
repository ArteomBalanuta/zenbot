package tool

import (
	"context"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/core"
)

func TestModerationActionDescriptorIsClosedAndRejectsTargetFields(t *testing.T) {
	tool := ModerationAction{Gateway: commandgateway.NewModerationGateway(&moderationToolOperator{}), Target: core.ModerationTarget{Name: "alice"}}
	ctx, _ := api.NewContextWithCapabilities("room", "bot", "creator", "", false, []string{}, []api.Capability{api.ModerationCommands})
	d, err := tool.Descriptor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.ValidateArguments(d.Parameters(), []byte(`{"action":"mute","target":"other"}`)); err == nil {
		t.Fatal("target field accepted")
	}
	result, err := tool.Execute(context.Background(), ctx, []byte(`{"action":"mute","target":"other"}`))
	if err != nil || !result.IsError {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestModerationActionReturnsVerifiedCommittedOutcome(t *testing.T) {
	action := ModerationAction{Gateway: commandgateway.NewModerationGateway(&moderationToolOperator{}), Target: core.ModerationTarget{Name: "alice"}}
	ctx, _ := api.NewContextWithCapabilities("room", "bot", "creator", "", false, []string{}, []api.Capability{api.ModerationCommands})

	result, err := action.Execute(context.Background(), ctx, []byte(`{"action":"kick"}`))
	if err != nil || result.IsError {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if !result.EffectsCommitted {
		t.Fatal("successful moderation action lacks verified committed-effect metadata")
	}
}

type moderationToolOperator struct{}

func (moderationToolOperator) EnableCaptcha(context.Context) error                       { return nil }
func (moderationToolOperator) MutePrincipal(context.Context, string) error               { return nil }
func (moderationToolOperator) UnmuteTarget(context.Context, core.ModerationTarget) error { return nil }
func (moderationToolOperator) KickPrincipal(context.Context, string) error               { return nil }
func (moderationToolOperator) ShadowBan(context.Context, string) error                   { return nil }
func (moderationToolOperator) UnshadowBanTarget(context.Context, core.ModerationTarget) error {
	return nil
}
