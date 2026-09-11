package tool

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/tool/contract"
	commandcatalog "zenbot/internal/command/catalog"
)

const runCommandName = "run_command"

const baseCommandResultSchemaJSON = `{"type":"object","additionalProperties":false,"properties":{"messages":{"type":"array","items":{"type":"string"}},"deliveredCount":{"type":"integer"},"actionCount":{"type":"integer"}},"required":["messages","deliveredCount","actionCount"]}`
const listCommandResultSchemaJSON = `{"type":"object","additionalProperties":false,"properties":{"messages":{"type":"array","items":{"type":"string"}},"deliveredCount":{"type":"integer"},"actionCount":{"type":"integer"},"data":{"type":"object","additionalProperties":false,"properties":{"room":{"type":"string"},"users":{"type":"array","items":{"type":"string"}},"count":{"type":"integer"},"returnedCount":{"type":"integer"},"truncated":{"type":"boolean"}},"required":["room","users","count","returnedCount","truncated"]}},"required":["messages","deliveredCount","actionCount"]}`
const runCommandResultSchemaJSON = `{"type":"object","additionalProperties":false,"properties":{"messages":{"type":"array","items":{"type":"string"}},"deliveredCount":{"type":"integer"},"actionCount":{"type":"integer"},"data":{"type":"any"}},"required":["messages","deliveredCount","actionCount"]}`

func commandResultSchema(schema string) json.RawMessage {
	return append(json.RawMessage(nil), schema...)
}

// RunCommand exposes a compact capability-aware command subset through the trusted command gateway.
type RunCommand struct{ Gateway commandgateway.Gateway }

func (t RunCommand) Name() string { return runCommandName }

func (t RunCommand) Descriptor(caller api.Context) (contract.Descriptor, error) {
	parameters, err := json.Marshal(map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"command":   map[string]any{"type": "string", "enum": runCommandAliases(caller)},
			"arguments": map[string]any{"type": "string", "maxLength": 4000},
		},
		"required": []string{"command"},
	})
	if err != nil {
		return contract.Descriptor{}, err
	}
	result := commandResultSchema(runCommandResultSchemaJSON)
	return contract.NewDescriptor(runCommandName, "Run Saturn command", "Run one capability-approved Saturn informational or moderation command and return its successfully delivered output. Invoke this tool immediately when the newest user request asks to run an enum-listed command; successful output is delivered directly to the room. Do not answer with instructions, a command snippet, a promise, or simulated output instead of invoking it. Commands always execute in provider order.", "commands", contract.AccessUser, contract.Action, contract.RoomDelivery, parameters, nil, nil, false, 10*time.Second, result, nil, []string{"commands", "room_delivery"}, []string{"Do not use for commands absent from the contextual enum or when no command execution is requested.", "Do not run this action concurrently with another command."})
}

func (t RunCommand) Execute(ctx context.Context, caller api.Context, args json.RawMessage) (contract.Result, error) {
	if err := ctx.Err(); err != nil {
		return contract.ActionErrorResult("", t.Name(), "TOOL_BATCH_CANCELLED", "command was cancelled before execution", contract.EffectNotStarted), nil
	}
	if t.Gateway == nil {
		return contract.ActionErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "command gateway is unavailable", contract.EffectNotStarted), nil
	}
	var input struct {
		Command   string `json:"command"`
		Arguments string `json:"arguments"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return contract.ActionErrorResult("", t.Name(), "INVALID_ARGUMENTS", "invalid run command arguments", contract.EffectNotStarted), nil
	}
	name := strings.ToLower(strings.TrimSpace(input.Command))
	if !containsCommandAlias(runCommandAliases(caller), name) {
		return contract.ActionErrorResult("", t.Name(), "COMMAND_REJECTED", "command is not allowed", contract.EffectNotStarted), nil
	}
	arguments := strings.TrimSpace(input.Arguments)
	if target := caller.ModerationTarget(); target != nil && commandcatalog.TargetsUser(name) && !sameModerationTarget(firstArgument(arguments), *target) {
		return contract.ActionErrorResult("", t.Name(), "COMMAND_REJECTED", "moderation action must target the reviewed author", contract.EffectNotStarted), nil
	}
	executed, err := t.Gateway.Execute(ctx, caller, name, arguments)
	if err != nil && executed.Status != commandgateway.OutcomeRejected && executed.Status != commandgateway.OutcomeNotFound {
		executed.Status = commandgateway.OutcomeUnknown
	}
	if failure, rejected := commandExecutionFailure(t.Name(), executed, false); rejected {
		return failure, nil
	}
	return commandExecutionSuccess(t.Name(), executed), nil
}

func commandExecutionSuccess(toolName string, execution commandgateway.Execution) contract.Result {
	deliveryCount := 0
	if execution.Delivery != nil {
		deliveryCount = execution.Delivery.Count
	}
	actionCount := 0
	if execution.Action != nil {
		actionCount = execution.Action.Count
	}
	messages := append([]string{}, execution.Messages...)
	payload := map[string]any{"messages": messages, "deliveredCount": deliveryCount, "actionCount": actionCount}
	if len(execution.Data) > 0 {
		payload["data"] = append(json.RawMessage(nil), execution.Data...)
	}
	result := contract.ActionSuccessResult("", toolName, payload, deliveryCount)
	result.ActionCount = actionCount
	return result
}

func commandExecutionFailure(toolName string, execution commandgateway.Execution, allowsSilentAction bool) (contract.Result, bool) {
	state := contract.EffectNotCommitted
	if execution.EffectsCommitted || (execution.Action != nil && execution.Action.Count > 0) || (execution.Delivery != nil && execution.Delivery.Count > 0) {
		state = contract.EffectPartial
	}
	failure := func(code, message string, effect contract.EffectState) (contract.Result, bool) {
		if len(execution.Messages) > 0 {
			message += "; output already delivered: " + strings.Join(execution.Messages, "\n")
		}
		result := contract.ActionErrorResult("", toolName, code, message, effect)
		result.EffectsCommitted = execution.EffectsCommitted || state == contract.EffectPartial
		if execution.Action != nil {
			result.ActionCount = execution.Action.Count
		}
		if execution.Delivery != nil {
			result.DeliveryCount = execution.Delivery.Count
		}
		return result, true
	}
	switch execution.Status {
	case commandgateway.OutcomeUnknown:
		return failure("ACTION_OUTCOME_UNKNOWN", "action outcome is unknown; do not repeat it", contract.EffectUnknown)
	case commandgateway.OutcomeNotFound:
		return failure("NOT_FOUND", "requested record was not found at command execution time; earlier reads may now be stale, so reassess using current evidence or corrected arguments before retrying", state)
	case commandgateway.OutcomeSucceeded:
		if !execution.EffectsCommitted || execution.Action == nil || execution.Action.Count <= 0 {
			return failure("UNVERIFIED_ACTION_OUTCOME", "command completion was not verified", contract.EffectUnknown)
		}
		if !allowsSilentAction && (execution.Delivery == nil || execution.Delivery.Count <= 0) {
			return failure("UNVERIFIED_ROOM_DELIVERY", "command executed but output delivery was not verified", contract.EffectCommitted)
		}
		return contract.Result{}, false
	case commandgateway.OutcomeRejected:
		return failure("COMMAND_REJECTED", "command was rejected; correct its arguments or choose another tool", state)
	default:
		return failure("ACTION_OUTCOME_UNKNOWN", "command returned no verified outcome; do not repeat it", contract.EffectUnknown)
	}
}

func runCommandAliases(caller api.Context) []string {
	return commandcatalog.RunCommandAliases(caller.HasCapability(api.ModerationCommands), caller.HasCapability(api.PermanentBan))
}

func containsCommandAlias(aliases []string, name string) bool {
	for _, alias := range aliases {
		if alias == name {
			return true
		}
	}
	return false
}

var _ Tool = RunCommand{}
