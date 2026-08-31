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
)

const runCommandName = "run_command"

// RunCommand exposes only fixed public informational aliases through the trusted command gateway.
type RunCommand struct{ Gateway commandgateway.Gateway }

func (t RunCommand) Name() string { return runCommandName }

func (t RunCommand) Descriptor(api.Context) (contract.Descriptor, error) {
	parameters := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"command":{"type":"string","enum":["h","help","info","list","p","ping","t","time","users","v","version","w","weather"]},"arguments":{"type":"string","maxLength":4000}},"required":["command"]}`)
	result := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"messages":{"type":"array","items":{"type":"string"}},"deliveredCount":{"type":"integer"}},"required":["messages","deliveredCount"]}`)
	return contract.NewDescriptor(runCommandName, "Run public command", "Run one fixed public informational command and return only its successfully delivered output.", "commands", contract.AccessUser, contract.Action, contract.RoomDelivery, parameters, nil, nil, false, 10*time.Second, result, nil, []string{"commands", "room_delivery"}, []string{"Do not use for moderation, private commands, or aliases outside the fixed public list."})
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
	if !runCommandAlias(name) {
		return contract.ErrorResult("", t.Name(), "COMMAND_REJECTED", "command is not allowed"), nil
	}
	executed, err := t.Gateway.Execute(ctx, caller, name, strings.TrimSpace(input.Arguments))
	if err != nil {
		return contract.Result{}, err
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

func runCommandAlias(name string) bool {
	switch name {
	case "help", "h", "list", "users", "info", "ping", "p", "weather", "w", "time", "t", "version", "v":
		return true
	default:
		return false
	}
}

var _ Tool = RunCommand{}
