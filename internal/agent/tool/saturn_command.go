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
	access := contract.AccessUser
	if required, restricted := requiredCommandCapability(definition); restricted {
		capabilities = []string{string(required)}
		if required == api.AdminCommands {
			access = contract.AccessAdmin
		} else {
			access = contract.AccessModerator
		}
	}
	aliases := append([]string(nil), definition.Aliases...)
	description := fmt.Sprintf("Execute Saturn's '%s' command (aliases: %s). Pass only the exact argument text that follows the command alias; Saturn performs command validation and room delivery.", definition.Canonical, strings.Join(aliases, ", "))
	parameters := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"arguments":{"type":"string","maxLength":4000,"description":"Exact text after the Saturn command alias, without the prefix."}},"required":[]}`)
	result := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"messages":{"type":"array","items":{"type":"string"}},"deliveredCount":{"type":"integer"}},"required":["messages","deliveredCount"]}`)
	writes := []string{"commands", "room_delivery"}
	if access == contract.AccessModerator {
		writes = append(writes, "moderation")
	}
	return contract.NewDescriptor(t.Name(), "Run Saturn "+definition.Canonical, description, "commands", access, contract.Action, contract.RoomDelivery, parameters, capabilities, nil, false, 10*time.Second, result, nil, writes, []string{
		"Do not use when the user is asking a question that does not require executing this command.",
		"Do not invent arguments or use this tool for a different Saturn command.",
	})
}

func (t SaturnCommand) Execute(ctx context.Context, caller api.Context, args json.RawMessage) (contract.Result, error) {
	if err := ctx.Err(); err != nil {
		return contract.Result{}, err
	}
	definition, ok := commandcatalog.AgentEntry(t.Definition.Canonical)
	if !ok {
		return contract.ErrorResult("", t.Name(), "UNKNOWN_TOOL", "Saturn command is unavailable"), nil
	}
	if !agentCommandAuthorized(caller, definition) {
		return contract.ErrorResult("", t.Name(), "TOOL_NOT_AUTHORIZED", "Caller is not allowed to execute this Saturn command"), nil
	}
	if t.Gateway == nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "command gateway is unavailable"), nil
	}
	descriptor, err := t.Descriptor(caller)
	if err != nil {
		return contract.Result{}, err
	}
	if err := contract.ValidateArguments(descriptor.Parameters(), args); err != nil {
		return contract.ErrorResult("", t.Name(), "INVALID_ARGUMENTS", err.Error()), nil
	}
	var input struct {
		Arguments string `json:"arguments"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return contract.ErrorResult("", t.Name(), "INVALID_ARGUMENTS", "arguments must be an object"), nil
		}
	}
	arguments := strings.TrimSpace(input.Arguments)
	if target := caller.ModerationTarget(); target != nil && commandcatalog.TargetsUser(definition.Canonical) && !sameModerationTarget(firstArgument(arguments), *target) {
		return contract.ErrorResult("", t.Name(), "COMMAND_REJECTED", "moderation action must target the reviewed author"), nil
	}
	execution, err := t.Gateway.Execute(ctx, caller, definition.Canonical, arguments)
	if err != nil {
		if ctx.Err() != nil {
			return contract.Result{}, ctx.Err()
		}
		return contract.ErrorResult("", t.Name(), "COMMAND_REJECTED", "Saturn command could not run"), nil
	}
	if !execution.Executed {
		return contract.ErrorResult("", t.Name(), "COMMAND_REJECTED", "Saturn rejected the command invocation"), nil
	}
	messages := append([]string(nil), execution.Messages...)
	if len(messages) == 0 {
		messages = []string{fmt.Sprintf("Saturn command '%s' executed and delivered any command output directly to the room.", definition.Canonical)}
	}
	return contract.SuccessResult("", t.Name(), map[string]any{"messages": messages, "deliveredCount": len(execution.Messages)}), nil
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
