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

const runCommandName = "run_command"

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
	result := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"messages":{"type":"array","items":{"type":"string"}},"deliveredCount":{"type":"integer"}},"required":["messages","deliveredCount"]}`)
	return contract.NewDescriptor(runCommandName, "Run Saturn command", "Run one capability-approved Saturn informational or moderation command and return its successfully delivered output. Invoke this tool immediately when the newest user request asks to run an enum-listed command; successful output is delivered directly to the room. Do not answer with instructions, a command snippet, a promise, or simulated output instead of invoking it. Commands always execute in provider order.", "commands", contract.AccessUser, contract.Action, contract.RoomDelivery, parameters, nil, nil, false, 10*time.Second, result, nil, []string{"commands", "room_delivery"}, []string{"Do not use for commands absent from the contextual enum or when no command execution is requested.", "Do not run this action concurrently with another command."})
}

func (t RunCommand) Execute(ctx context.Context, caller api.Context, args json.RawMessage) (contract.Result, error) {
	if err := ctx.Err(); err != nil {
		return contract.Result{}, err
	}
	if t.Gateway == nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "command gateway is unavailable"), nil
	}
	var input struct {
		Command   string `json:"command"`
		Arguments string `json:"arguments"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return contract.Result{}, fmt.Errorf("invalid run command arguments")
	}
	name := strings.ToLower(strings.TrimSpace(input.Command))
	if !containsCommandAlias(runCommandAliases(caller), name) {
		return contract.ErrorResult("", t.Name(), "COMMAND_REJECTED", "command is not allowed"), nil
	}
	arguments := strings.TrimSpace(input.Arguments)
	if target := caller.ModerationTarget(); target != nil && commandcatalog.TargetsUser(name) && !sameModerationTarget(firstArgument(arguments), *target) {
		return contract.ErrorResult("", t.Name(), "COMMAND_REJECTED", "moderation action must target the reviewed author"), nil
	}
	executed, err := t.Gateway.Execute(ctx, caller, name, arguments)
	if err != nil {
		if ctx.Err() != nil {
			return contract.Result{}, ctx.Err()
		}
		return contract.ErrorResult("", t.Name(), "COMMAND_REJECTED", "Saturn command could not run"), nil
	}
	if !executed.Executed {
		return contract.ErrorResult("", t.Name(), "COMMAND_REJECTED", "command was rejected"), nil
	}
	messages := append([]string(nil), executed.Messages...)
	if len(messages) == 0 {
		messages = []string{fmt.Sprintf("Saturn command '%s' executed; its output was sent to the room. No other Saturn command was executed.", name)}
	}
	return contract.SuccessResult("", t.Name(), map[string]any{"messages": messages, "deliveredCount": len(executed.Messages)}), nil
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
