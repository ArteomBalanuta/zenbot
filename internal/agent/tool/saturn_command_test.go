package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	commandcatalog "zenbot/internal/command/catalog"
)

func agentCommandDefinition(t *testing.T, canonical string) commandcatalog.Entry {
	t.Helper()
	definition, ok := commandcatalog.AgentEntry(canonical)
	if ok {
		return definition
	}
	t.Fatalf("agent command definition %q not found", canonical)
	return commandcatalog.Entry{}
}

func TestSaturnCommandDescriptorCarriesCatalogIdentityAndCapabilityPolicy(t *testing.T) {
	public, _ := api.NewContext("room", "caller", "", "", false, []string{})
	weather := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "weather")}
	descriptor, err := weather.Descriptor(public)
	if err != nil || weather.Name() != "saturn_weather" || descriptor.Effect() != contract.Action || descriptor.ResultMode() != contract.RoomDelivery || descriptor.Idempotent() || len(descriptor.RequiredCapabilities()) != 0 {
		t.Fatalf("weather descriptor=%#v err=%v", descriptor, err)
	}
	if !strings.Contains(descriptor.Description(), "aliases") || !strings.Contains(descriptor.Description(), "today") {
		t.Fatalf("weather description=%q", descriptor.Description())
	}

	prefix := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "prefix")}
	descriptor, err = prefix.Descriptor(public)
	if err != nil || len(descriptor.RequiredCapabilities()) != 1 || descriptor.RequiredCapabilities()[0] != string(api.AdminCommands) {
		t.Fatalf("prefix descriptor=%#v err=%v", descriptor, err)
	}
}

func TestSaturnCommandDispatchesCanonicalArgumentsAndEnforcesCapabilities(t *testing.T) {
	gateway := &runCommandGatewayStub{result: commandgateway.Execution{Executed: true, Messages: []string{"sent"}}}
	creator, _ := api.NewContextWithCapabilities("room", "creator", "trip", "", false, []string{}, []api.Capability{api.AdminCommands})
	tool := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "prefix"), Gateway: gateway}
	result, err := tool.Execute(context.Background(), creator, json.RawMessage(`{"arguments":"  $  "}`))
	if err != nil || result.IsError || gateway.calls != 1 || gateway.command != "prefix" || gateway.arguments != "$" {
		t.Fatalf("result=%#v gateway=%#v err=%v", result, gateway, err)
	}
	public, _ := api.NewContext("room", "caller", "", "", false, []string{})
	result, err = tool.Execute(context.Background(), public, json.RawMessage(`{}`))
	if err != nil || !result.IsError || result.ErrorCode != "TOOL_NOT_AUTHORIZED" || gateway.calls != 1 {
		t.Fatalf("unauthorized result=%#v calls=%d err=%v", result, gateway.calls, err)
	}
}

func TestSaturnModerationCommandCannotRetargetReviewedAuthor(t *testing.T) {
	gateway := &runCommandGatewayStub{result: commandgateway.Execution{Executed: true}}
	moderator, _ := api.NewContextWithModerationTarget("room", "bot", "creator", "", false, []string{}, []api.Capability{api.ModerationCommands}, "alice")
	tool := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "kick"), Gateway: gateway}
	result, err := tool.Execute(context.Background(), moderator, json.RawMessage(`{"arguments":"bob"}`))
	if err != nil || !result.IsError || result.ErrorCode != "COMMAND_REJECTED" || gateway.calls != 0 {
		t.Fatalf("result=%#v calls=%d err=%v", result, gateway.calls, err)
	}
}

func TestSaturnCommandTurnsGatewayRejectionIntoCorrectableObservation(t *testing.T) {
	gateway := &runCommandGatewayStub{err: errors.New("command validation failed")}
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})
	tool := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "weather"), Gateway: gateway}
	result, err := tool.Execute(context.Background(), caller, json.RawMessage(`{"arguments":"Chisinau"}`))
	if err != nil || !result.IsError || result.ErrorCode != "COMMAND_REJECTED" || gateway.calls != 1 {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, gateway.calls)
	}
}
