package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/tool/contract"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
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

func (t SaturnCommand) Authorized(caller api.Context) bool {
	definition, ok := commandcatalog.AgentEntry(t.Definition.Canonical)
	if !ok || !commandgateway.Authorized(caller, definition) || t.Gateway == nil {
		return false
	}
	if availability, ok := t.Gateway.(commandgateway.CanonicalAvailability); ok {
		return availability.CommandAvailable(definition.Canonical)
	}
	return true
}

func (t SaturnCommand) Descriptor(api.Context) (contract.Descriptor, error) {
	definition, ok := commandcatalog.AgentEntry(t.Definition.Canonical)
	if !ok {
		return contract.Descriptor{}, fmt.Errorf("unknown Saturn command tool %q", t.Definition.Canonical)
	}
	capabilities := []string(nil)
	access := contractAccess(definition.Agent.Access)
	if required, restricted := commandgateway.RequiredCapability(definition); restricted {
		capabilities = []string{string(required)}
	}
	resultSchema := baseCommandResultSchemaJSON
	if definition.Canonical == "list" {
		resultSchema = listCommandResultSchemaJSON
	} else if definition.Canonical == "restart" || definition.Canonical == "shutdown" {
		resultSchema = lifecycleCommandResultSchemaJSON
	}
	result := commandResultSchema(resultSchema)
	resultMode := contract.RoomDelivery
	writes := []string{"commands", "room_delivery"}
	if definition.Agent.AllowsSilentAction {
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
	if !commandgateway.Authorized(caller, definition) {
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
	if !commandgateway.TargetAllowed(caller, definition.Canonical, arguments) {
		return contract.ActionErrorResult("", t.Name(), "COMMAND_REJECTED", "moderation action must target the reviewed author", contract.EffectNotStarted), nil
	}
	execution, err := t.Gateway.Execute(ctx, caller, definition.Canonical, arguments)
	var notAdmitted *common.LifecycleRequestRejectedError
	if execution.Status == commandgateway.OutcomeRejected && errors.As(err, &notAdmitted) &&
		(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) &&
		!execution.EffectsCommitted && !execution.DataObserved &&
		(execution.Action == nil || execution.Action.Count == 0) && (execution.Delivery == nil || execution.Delivery.Count == 0) {
		return contract.ActionErrorResult("", t.Name(), "TOOL_BATCH_CANCELLED", "lifecycle request was cancelled before admission", contract.EffectNotStarted), nil
	}
	if err != nil && execution.Status != commandgateway.OutcomeRejected && execution.Status != commandgateway.OutcomeNotFound && execution.Status != commandgateway.OutcomeAmbiguous {
		execution.Status = commandgateway.OutcomeUnknown
	}
	// Optional source facts must satisfy the closed command result contract on
	// success as well as failure. Invalid facts do not erase action receipts.
	if len(execution.Data) > 0 && contract.ValidateResult(descriptor.ResultSchema(), []byte(commandExecutionSuccess(t.Name(), execution).Content)) != nil {
		execution.Data = nil
		execution.DataObserved = false
	}
	if failure, rejected := commandExecutionFailure(t.Name(), execution, definition.Agent.AllowsSilentAction); rejected {
		return failure.ValidateObservedData(descriptor.ResultSchema()), nil
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

var _ Tool = SaturnCommand{}
