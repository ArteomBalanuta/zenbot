package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/command"
)

type runCommandGatewayStub struct {
	calls              int
	command, arguments string
	result             command.CommandExecution
	err                error
}

func (s *runCommandGatewayStub) Execute(_ context.Context, _ api.Context, commandName, arguments string) (command.CommandExecution, error) {
	s.calls++
	s.command, s.arguments = commandName, arguments
	return s.result, s.err
}

func TestRunCommandDescriptorIsClosedBoundedAction(t *testing.T) {
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})
	d, err := (agenttool.RunCommand{}).Descriptor(caller)
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "run_command" || d.Effect() != contract.Action || d.ResultMode() != contract.RoomDelivery || d.Idempotent() || d.Timeout() != 10*time.Second {
		t.Fatalf("descriptor=%#v", d)
	}
	if !strings.Contains(d.Description(), "Invoke this tool immediately") || !strings.Contains(d.Description(), "Do not answer with instructions") {
		t.Fatalf("execution guidance missing from description: %q", d.Description())
	}
	var parameters struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if json.Unmarshal(d.Parameters(), &parameters) != nil || !slices.Contains(parameters.Properties["command"].Enum, "lastseen") || slices.Contains(parameters.Properties["command"].Enum, "mute") {
		t.Fatalf("parameters=%s", d.Parameters())
	}
	if string(d.ResultSchema()) != `{"type":"object","additionalProperties":false,"properties":{"messages":{"type":"array","items":{"type":"string"}},"deliveredCount":{"type":"integer"}},"required":["messages","deliveredCount"]}` {
		t.Fatalf("result=%s", d.ResultSchema())
	}
}

func TestRunCommandDescriptorAddsModerationAndPermanentBanAliasesByCapability(t *testing.T) {
	moderator, _ := api.NewContextWithCapabilities("room", "moderator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
	d, err := (agenttool.RunCommand{}).Descriptor(moderator)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if json.Unmarshal(d.Parameters(), &schema) != nil || !slices.Contains(schema.Properties["command"].Enum, "kick") || slices.Contains(schema.Properties["command"].Enum, "ban") {
		t.Fatalf("moderator schema=%s", d.Parameters())
	}
	creator, _ := api.NewContextWithCapabilities("room", "creator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands, api.PermanentBan})
	d, err = (agenttool.RunCommand{}).Descriptor(creator)
	if err != nil || !strings.Contains(string(d.Parameters()), `"ban"`) {
		t.Fatalf("creator schema=%s err=%v", d.Parameters(), err)
	}
}

func TestRunCommandNormalizesCallsGatewayOnceAndRejectsFailure(t *testing.T) {
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})
	gateway := &runCommandGatewayStub{result: command.CommandExecution{Executed: true, Messages: []string{"forecast"}}}
	result, err := (agenttool.RunCommand{Gateway: gateway}).Execute(context.Background(), caller, json.RawMessage(`{"command":" W ","arguments":" Tokyo "}`))
	var body struct {
		Messages       []string `json:"messages"`
		DeliveredCount int      `json:"deliveredCount"`
	}
	if json.Unmarshal([]byte(result.Content), &body) != nil || len(body.Messages) != 1 || body.Messages[0] != "forecast" || body.DeliveredCount != 1 {
		t.Fatalf("result content=%s", result.Content)
	}
	if err != nil || result.IsError || gateway.calls != 1 || gateway.command != "w" || gateway.arguments != "Tokyo" {
		t.Fatalf("result=%#v err=%v gateway=%#v", result, err, gateway)
	}
	gateway = &runCommandGatewayStub{result: command.CommandExecution{Executed: false}}
	result, err = (agenttool.RunCommand{Gateway: gateway}).Execute(context.Background(), caller, json.RawMessage(`{"command":"ping"}`))
	if err != nil || !result.IsError || result.ErrorCode != "COMMAND_REJECTED" || gateway.calls != 1 {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, gateway.calls)
	}
}

func TestRunCommandTurnsGatewayRejectionIntoCorrectableObservation(t *testing.T) {
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})
	gateway := &runCommandGatewayStub{err: errors.New("command validation failed")}
	result, err := (agenttool.RunCommand{Gateway: gateway}).Execute(context.Background(), caller, json.RawMessage(`{"command":"ping"}`))
	if err != nil || !result.IsError || result.ErrorCode != "COMMAND_REJECTED" || gateway.calls != 1 {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, gateway.calls)
	}
}

func TestRunCommandModerationAliasCannotRetargetReviewedAuthor(t *testing.T) {
	moderator, _ := api.NewContextWithModerationTarget("room", "bot", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands}, "alice")
	gateway := &runCommandGatewayStub{result: command.CommandExecution{Executed: true}}
	result, err := (agenttool.RunCommand{Gateway: gateway}).Execute(context.Background(), moderator, json.RawMessage(`{"command":"k","arguments":"bob"}`))
	if err != nil || !result.IsError || result.ErrorCode != "COMMAND_REJECTED" || gateway.calls != 0 {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, gateway.calls)
	}
}
