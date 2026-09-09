package tool

import (
	"context"
	"encoding/json"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/core"
)

const moderationActionName = "moderation_action"

// ModerationAction has no model-supplied target or arbitrary command arguments.
type ModerationAction struct {
	Gateway *commandgateway.ModerationGateway
	Target  core.ModerationTarget
}

func (t ModerationAction) Name() string { return moderationActionName }

func (t ModerationAction) Descriptor(api.Context) (contract.Descriptor, error) {
	parameters := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"action":{"type":"string","enum":["captcha","mute","unmute","kick","shadowban","unshadowban"]}},"required":["action"]}`)
	result := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"executed":{"type":"boolean"}},"required":["executed"]}`)
	return contract.NewDescriptor(moderationActionName, "Perform one moderation action", "Perform one fixed moderation action against the listener-resolved target.", "moderation", contract.AccessUser, contract.Action, contract.ModelData, parameters, nil, nil, false, 10*time.Second, result, nil, []string{"moderation"}, []string{"Never supply a target, arguments, hash, nick, or reason."})
}

func (t ModerationAction) Execute(ctx context.Context, caller api.Context, args json.RawMessage) (contract.Result, error) {
	if ctx == nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "moderation context is nil"), nil
	}
	if err := ctx.Err(); err != nil {
		return contract.Result{}, err
	}
	d, err := t.Descriptor(caller)
	if err != nil {
		return contract.Result{}, err
	}
	if err := contract.ValidateArguments(d.Parameters(), args); err != nil {
		return contract.ErrorResult("", t.Name(), "COMMAND_REJECTED", "invalid moderation action"), nil
	}
	var input struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return contract.ErrorResult("", t.Name(), "COMMAND_REJECTED", "invalid moderation action"), nil
	}
	action, err := commandgateway.ParseModerationAction(input.Action)
	if err != nil {
		return contract.ErrorResult("", t.Name(), "COMMAND_REJECTED", "invalid moderation action"), nil
	}
	if t.Gateway == nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "moderation gateway is unavailable"), nil
	}
	if err := t.Gateway.Execute(ctx, api.MODERATION, caller, t.Target, action); err != nil {
		return contract.ErrorResult("", t.Name(), "COMMAND_REJECTED", "moderation action was rejected"), nil
	}
	return contract.SuccessResult("", t.Name(), map[string]any{"executed": true}), nil
}

var _ Tool = ModerationAction{}
