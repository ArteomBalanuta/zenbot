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
	if descriptor.Label() != "Current weather" || descriptor.Description() != "Fetch and display current weather for a location." {
		t.Fatalf("weather identity metadata=%q / %q", descriptor.Label(), descriptor.Description())
	}
	routing := descriptor.Routing()
	if !strings.Contains(strings.Join(routing.Aliases, ","), "today") || len(routing.UseWhen) == 0 || len(routing.Examples) == 0 {
		t.Fatalf("weather routing metadata=%#v", routing)
	}
	if err := contract.ValidateArguments(descriptor.Parameters(), json.RawMessage(`{"location":"Chisinau"}`)); err != nil {
		t.Fatalf("typed weather arguments rejected: %v", err)
	}
	if err := contract.ValidateArguments(descriptor.Parameters(), json.RawMessage(`{"arguments":"Chisinau"}`)); err == nil {
		t.Fatal("legacy generic arguments unexpectedly accepted")
	}
	list := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "list")}
	listDescriptor, err := list.Descriptor(public)
	if err != nil || !strings.Contains(strings.Join(listDescriptor.Routing().UseWhen, " "), "currently in a room") || !strings.Contains(strings.Join(listDescriptor.ResourceWrites(), " "), "room_delivery") {
		t.Fatalf("remote-room list guidance missing: %#v err=%v", listDescriptor.Routing(), err)
	}

	prefix := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "prefix")}
	descriptor, err = prefix.Descriptor(public)
	if err != nil || len(descriptor.RequiredCapabilities()) != 1 || descriptor.RequiredCapabilities()[0] != string(api.AdminCommands) {
		t.Fatalf("prefix descriptor=%#v err=%v", descriptor, err)
	}
}

func TestSaturnCommandDispatchesCanonicalArgumentsAndEnforcesCapabilities(t *testing.T) {
	gateway := &runCommandGatewayStub{result: verifiedCommandExecution("sent")}
	creator, _ := api.NewContextWithCapabilities("room", "creator", "trip", "", false, []string{}, []api.Capability{api.AdminCommands})
	tool := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "prefix"), Gateway: gateway}
	result, err := tool.Execute(context.Background(), creator, json.RawMessage(`{"prefix":"$"}`))
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
	gateway := &runCommandGatewayStub{result: verifiedCommandExecution("not reached")}
	moderator, _ := api.NewContextWithModerationTarget("room", "bot", "creator", "", false, []string{}, []api.Capability{api.ModerationCommands}, "alice")
	tool := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "kick"), Gateway: gateway}
	result, err := tool.Execute(context.Background(), moderator, json.RawMessage(`{"mode":"exact","targets":["bob"]}`))
	if err != nil || !result.IsError || result.ErrorCode != "COMMAND_REJECTED" || gateway.calls != 0 {
		t.Fatalf("result=%#v calls=%d err=%v", result, gateway.calls, err)
	}
}

func TestSaturnCommandTurnsGatewayRejectionIntoCorrectableObservation(t *testing.T) {
	gateway := &runCommandGatewayStub{err: errors.New("command validation failed")}
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})
	tool := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "weather"), Gateway: gateway}
	result, err := tool.Execute(context.Background(), caller, json.RawMessage(`{"location":"Chisinau"}`))
	if err != nil || !result.IsError || result.ErrorCode != "COMMAND_REJECTED" || gateway.calls != 1 {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, gateway.calls)
	}
}

func TestSaturnCommandRequiresVerifiedCommittedDelivery(t *testing.T) {
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})
	tests := []struct {
		name      string
		execution commandgateway.Execution
		wantCode  string
	}{
		{name: "rejected", execution: commandgateway.Execution{Status: commandgateway.OutcomeRejected}, wantCode: "COMMAND_REJECTED"},
		{name: "unknown", execution: commandgateway.Execution{Status: commandgateway.OutcomeUnknown}, wantCode: "ACTION_OUTCOME_UNKNOWN"},
		{name: "uncommitted", execution: commandgateway.Execution{Status: commandgateway.OutcomeSucceeded, Delivery: &commandgateway.DeliveryReceipt{Count: 1}}, wantCode: "UNVERIFIED_ACTION_OUTCOME"},
		{name: "missing receipt", execution: commandgateway.Execution{Status: commandgateway.OutcomeSucceeded, EffectsCommitted: true}, wantCode: "UNVERIFIED_ROOM_DELIVERY"},
		{name: "zero receipt", execution: commandgateway.Execution{Status: commandgateway.OutcomeSucceeded, EffectsCommitted: true, Delivery: &commandgateway.DeliveryReceipt{}}, wantCode: "UNVERIFIED_ROOM_DELIVERY"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &runCommandGatewayStub{result: tc.execution}
			tool := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "weather"), Gateway: gateway}

			result, err := tool.Execute(context.Background(), caller, json.RawMessage(`{"location":"Chisinau"}`))

			if err != nil || !result.IsError || result.ErrorCode != tc.wantCode || result.VerifiedRoomDelivery() {
				t.Fatalf("result=%#v err=%v, want %s", result, err, tc.wantCode)
			}
		})
	}

	tool := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "weather"), Gateway: &runCommandGatewayStub{result: verifiedCommandExecution("weather")}}
	result, err := tool.Execute(context.Background(), caller, json.RawMessage(`{"location":"Chisinau"}`))
	if err != nil || result.IsError || !result.VerifiedRoomDelivery() || result.DeliveryCount != 1 {
		t.Fatalf("verified result=%#v err=%v", result, err)
	}
}

func TestSaturnCommandRejectsCrossFieldArgumentsBeforeGateway(t *testing.T) {
	gateway := &runCommandGatewayStub{result: verifiedCommandExecution("not reached")}
	moderator, _ := api.NewContextWithCapabilities("room", "moderator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
	tool := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "kick"), Gateway: gateway}
	result, err := tool.Execute(context.Background(), moderator, json.RawMessage(`{"mode":"contains","targets":["raid","spam"]}`))
	if err != nil || !result.IsError || result.ErrorCode != "INVALID_ARGUMENTS" || gateway.calls != 0 {
		t.Fatalf("result=%#v calls=%d err=%v", result, gateway.calls, err)
	}
}

func TestEverySaturnCommandDescriptorUsesItsCatalogArgumentSchema(t *testing.T) {
	caller, _ := api.NewContextWithCapabilities("room", "creator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands, api.PermanentBan, api.AdminCommands})
	for _, definition := range commandcatalog.AgentEntries() {
		t.Run(definition.Canonical, func(t *testing.T) {
			tool := agenttool.SaturnCommand{Definition: definition}
			descriptor, err := tool.Descriptor(caller)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := string(descriptor.Parameters()), string(definition.Agent.Arguments.Schema()); got != want {
				t.Fatalf("parameter schema mismatch\nwant: %s\n got: %s", want, got)
			}
			for _, example := range definition.Agent.Examples {
				if err := contract.ValidateArguments(descriptor.Parameters(), example.Arguments); err != nil {
					t.Fatalf("example %q rejected: %v", example.Prompt, err)
				}
			}
		})
	}
}

func TestSaturnCommandEncodesRepresentativeCommandFamilies(t *testing.T) {
	public, _ := api.NewContext("room", "caller", "", "", false, []string{})
	moderator, _ := api.NewContextWithCapabilities("room", "moderator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
	creator, _ := api.NewContextWithCapabilities("room", "creator", "trip", "", false, []string{}, []api.Capability{api.AdminCommands})
	tests := []struct {
		name      string
		caller    api.Context
		canonical string
		arguments string
		wantTail  string
	}{
		{name: "weather phrase", caller: public, canonical: "weather", arguments: `{"location":"New York"}`, wantTail: "New York"},
		{name: "remote room list", caller: public, canonical: "list", arguments: `{"room":"lounge"}`, wantTail: "lounge"},
		{name: "moderator kick", caller: moderator, canonical: "kick", arguments: `{"mode":"multiple","targets":["@raider","spammer"]}`, wantTail: "-m @raider spammer"},
		{name: "access grant", caller: creator, canonical: "access", arguments: `{"trips":["aaa","bbb"],"role":"MODERATOR"}`, wantTail: "aaa,bbb MODERATOR"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gateway := &runCommandGatewayStub{result: verifiedCommandExecution("delivered")}
			command := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, test.canonical), Gateway: gateway}
			result, err := command.Execute(context.Background(), test.caller, json.RawMessage(test.arguments))
			if err != nil || result.IsError || gateway.calls != 1 || gateway.command != test.canonical || gateway.arguments != test.wantTail {
				t.Fatalf("result=%#v gateway=%#v err=%v", result, gateway, err)
			}
		})
	}
}
