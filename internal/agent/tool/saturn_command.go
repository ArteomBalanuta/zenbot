package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/tool/contract"
	commandcatalog "zenbot/internal/command/catalog"
)

// SaturnCommand adapts one entry from Saturn's production command registry to
// the agent tool protocol. The underlying command remains the source of
// execution semantics, validation, room delivery, and persistence behavior.
type SaturnCommand struct {
	Definition commandcatalog.Entry
	Gateway    commandgateway.Gateway
}

func (t SaturnCommand) Name() string {
	return "saturn_" + strings.ToLower(strings.TrimSpace(t.Definition.Canonical))
}

func (t SaturnCommand) Descriptor(api.Context) (contract.Descriptor, error) {
	definition, ok := commandcatalog.AgentEntry(t.Definition.Canonical)
	if !ok {
		return contract.Descriptor{}, fmt.Errorf("unknown Saturn command tool %q", t.Definition.Canonical)
	}
	capabilities := []string(nil)
	access := contractAccess(definition.Agent.Access)
	if required, restricted := requiredCommandCapability(definition); restricted {
		capabilities = []string{string(required)}
	}
	result := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"messages":{"type":"array","items":{"type":"string"}},"deliveredCount":{"type":"integer"},"actionCount":{"type":"integer"}},"required":["messages","deliveredCount","actionCount"]}`)
	resultMode := contract.RoomDelivery
	writes := []string{"commands", "room_delivery"}
	if definition.Canonical == "kick" {
		resultMode = contract.ModelData
		writes = []string{"commands"}
	}
	if definition.Agent.Access == commandcatalog.AgentModerator || definition.Agent.Access == commandcatalog.AgentPermanentBan {
		writes = append(writes, "moderation")
	}
	examples := make([]contract.Example, len(definition.Agent.Examples))
	for index, example := range definition.Agent.Examples {
		examples[index] = contract.Example{Prompt: example.Prompt, Arguments: append(json.RawMessage(nil), example.Arguments...)}
	}
	options := []contract.DescriptorOption{
		contract.WithRouting(contract.RoutingMetadata{
			Aliases:  append([]string(nil), definition.Aliases...),
			Targets:  append([]string(nil), definition.Agent.Targets...),
			UseWhen:  append([]string(nil), definition.Agent.UseWhen...),
			Examples: examples,
		}),
		contract.WithPrimaryIntent(definition.Agent.PrimaryIntent),
	}
	if definition.Agent.InternalFallback {
		options = append(options, contract.WithInternalFallback())
	}
	return contract.NewDescriptor(
		t.Name(), definition.Agent.Label, definition.Agent.Description, definition.Agent.Category,
		access, contract.Action, resultMode, definition.Agent.Arguments.Schema(),
		capabilities, nil, false, 10*time.Second, result, nil, writes, definition.Agent.WhenNotUse,
		options...,
	)
}

func (t SaturnCommand) Execute(ctx context.Context, caller api.Context, args json.RawMessage) (contract.Result, error) {
	if err := ctx.Err(); err != nil {
		return contract.ActionErrorResult("", t.Name(), "TOOL_BATCH_CANCELLED", "command was cancelled before execution", contract.EffectNotStarted), nil
	}
	definition, ok := commandcatalog.AgentEntry(t.Definition.Canonical)
	if !ok {
		return contract.ActionErrorResult("", t.Name(), "UNKNOWN_TOOL", "Saturn command is unavailable", contract.EffectNotStarted), nil
	}
	if !agentCommandAuthorized(caller, definition) {
		return contract.ActionErrorResult("", t.Name(), "TOOL_NOT_AUTHORIZED", "Caller is not allowed to execute this Saturn command", contract.EffectNotStarted), nil
	}
	if t.Gateway == nil {
		return contract.ActionErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "command gateway is unavailable", contract.EffectNotStarted), nil
	}
	descriptor, err := t.Descriptor(caller)
	if err != nil {
		return contract.ActionErrorResult("", t.Name(), "INVALID_TOOL_CONTRACT", "invalid command contract", contract.EffectNotStarted), nil
	}
	if err := contract.ValidateArguments(descriptor.Parameters(), args); err != nil {
		return contract.ActionErrorResult("", t.Name(), "INVALID_ARGUMENTS", err.Error(), contract.EffectNotStarted), nil
	}
	arguments, err := definition.Agent.Arguments.Encode(args)
	if err != nil {
		return contract.ActionErrorResult("", t.Name(), "INVALID_ARGUMENTS", err.Error(), contract.EffectNotStarted), nil
	}
	if definition.Canonical == "list" && strings.EqualFold(strings.TrimSpace(arguments), strings.TrimSpace(caller.Room())) {
		return contract.ActionErrorResult("", t.Name(), "INVALID_ARGUMENTS", "saturn_list requires a room other than the caller's current room; use room_users for current-room presence", contract.EffectNotStarted), nil
	}
	if target := caller.ModerationTarget(); target != nil && commandcatalog.TargetsUser(definition.Canonical) && !sameModerationTarget(firstArgument(arguments), *target) {
		return contract.ActionErrorResult("", t.Name(), "COMMAND_REJECTED", "moderation action must target the reviewed author", contract.EffectNotStarted), nil
	}
	execution, err := t.Gateway.Execute(ctx, caller, definition.Canonical, arguments)
	if err != nil && execution.Status != commandgateway.OutcomeRejected && execution.Status != commandgateway.OutcomeNotFound {
		execution.Status = commandgateway.OutcomeUnknown
	}
	if failure, rejected := commandExecutionFailure(t.Name(), execution, definition.Canonical == "kick"); rejected {
		return failure, nil
	}
	return commandExecutionSuccess(t.Name(), execution), nil
}

func contractAccess(access commandcatalog.AgentAccess) contract.Access {
	switch access {
	case commandcatalog.AgentAdmin:
		return contract.AccessAdmin
	case commandcatalog.AgentModerator, commandcatalog.AgentPermanentBan:
		return contract.AccessModerator
	default:
		return contract.AccessUser
	}
}

func requiredCommandCapability(definition commandcatalog.Entry) (api.Capability, bool) {
	switch commandcatalog.Access(definition) {
	case commandcatalog.AgentAdmin:
		return api.AdminCommands, true
	case commandcatalog.AgentModerator:
		return api.ModerationCommands, true
	case commandcatalog.AgentPermanentBan:
		return api.PermanentBan, true
	default:
		return "", false
	}
}

func agentCommandAuthorized(caller api.Context, definition commandcatalog.Entry) bool {
	required, restricted := requiredCommandCapability(definition)
	return !restricted || caller.HasCapability(required)
}

func firstArgument(arguments string) string {
	fields := strings.Fields(arguments)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func sameModerationTarget(argument, expected string) bool {
	return strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(argument), "@"), strings.TrimPrefix(strings.TrimSpace(expected), "@"))
}

var _ Tool = SaturnCommand{}
