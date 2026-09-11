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
	"zenbot/internal/agent/tool/execution"
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

func TestSaturnCommandErrorObservationRequiresMarkedDeclaredData(t *testing.T) {
	for _, tc := range []struct {
		name, data   string
		marked, want bool
	}{
		{"valid", `{"room":"lounge","count":0,"returnedCount":0,"truncated":false,"users":[]}`, true, true},
		{"unmarked", `{"room":"lounge","count":0,"returnedCount":0,"truncated":false,"users":[]}`, false, false},
		{"wrong shape", `{"private":"diagnostic"}`, true, false},
		{"malformed", `{"room":`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller, _ := api.NewContext("room", "caller", "", "", false, nil)
			gateway := &runCommandGatewayStub{result: commandgateway.Execution{Status: commandgateway.OutcomeUnknown, Data: json.RawMessage(tc.data), DataObserved: tc.marked}, err: errors.New("driver secret")}
			command := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "list"), Gateway: gateway}
			result, err := command.Execute(context.Background(), caller, json.RawMessage(`{"room":"lounge"}`))
			if err != nil || (len(result.ObservedData) > 0) != tc.want || result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || result.EffectState != contract.EffectUnknown || result.ActionCount != 0 || result.DeliveryCount != 0 || strings.Contains(string(result.Envelope()), "driver secret") {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if tc.want {
				descriptor, _ := command.Descriptor(caller)
				if err := contract.ValidateResult(descriptor.ResultSchema(), result.ObservedData); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
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
	if err != nil || listDescriptor.PrimaryIntent() != "remote_room_presence" || listDescriptor.InternalFallback() || !strings.Contains(strings.Join(listDescriptor.Routing().UseWhen, " "), "other than") || !strings.Contains(strings.Join(listDescriptor.ResourceWrites(), " "), "room_delivery") {
		t.Fatalf("remote-room list guidance missing: %#v err=%v", listDescriptor.Routing(), err)
	}

	prefix := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "prefix")}
	descriptor, err = prefix.Descriptor(public)
	if err != nil || len(descriptor.RequiredCapabilities()) != 1 || descriptor.RequiredCapabilities()[0] != string(api.AdminCommands) {
		t.Fatalf("prefix descriptor=%#v err=%v", descriptor, err)
	}
}

func TestSaturnKickDescriptorHasOneRootNicknameArgument(t *testing.T) {
	moderator, _ := api.NewContextWithCapabilities("room", "moderator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
	kick := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "kick")}
	descriptor, err := kick.Descriptor(moderator)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(descriptor.Parameters(), &schema); err != nil {
		t.Fatalf("decode provider schema: %v", err)
	}
	if schema.Type != "object" || len(schema.Properties) != 1 || schema.Properties["nick"] == nil || len(schema.Required) != 1 || schema.Required[0] != "nick" {
		t.Fatalf("kick provider schema = %s, want flat required nick object", descriptor.Parameters())
	}
	if err := contract.ValidateArguments(descriptor.Parameters(), json.RawMessage(`{"nick":"tajweed"}`)); err != nil {
		t.Fatalf("provider schema rejected saturn_kick({nick: tajweed}): %v", err)
	}
}

func TestSaturnPingAdvertisesItsFixedHackChatRuntimeTarget(t *testing.T) {
	public, _ := api.NewContext("programming", "caller", "", "", false, []string{})
	ping := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "ping")}
	descriptor, err := ping.Descriptor(public)
	if err != nil {
		t.Fatal(err)
	}
	routing := descriptor.Routing()
	if !strings.Contains(strings.ToLower(descriptor.Description()), "hack.chat:80") || !strings.Contains(strings.ToLower(strings.Join(routing.Targets, " ")), "hack.chat") {
		t.Fatalf("ping does not advertise its fixed runtime target: description=%q routing=%#v", descriptor.Description(), routing)
	}
	if err := contract.ValidateArguments(descriptor.Parameters(), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("argumentless ping was rejected: %v", err)
	}
	if err := contract.ValidateArguments(descriptor.Parameters(), json.RawMessage(`{"host":"example.com"}`)); err == nil {
		t.Fatal("ping accepted a caller-selected host")
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

func TestSaturnListRejectsCurrentRoomSoPresenceRoutesAreDisjoint(t *testing.T) {
	gateway := &runCommandGatewayStub{result: verifiedCommandExecution("not reached")}
	caller, _ := api.NewContext("Programming", "caller", "", "", false, []string{})
	list := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "list"), Gateway: gateway}

	result, err := list.Execute(context.Background(), caller, json.RawMessage(`{"room":" programming "}`))
	if err != nil || !result.IsError || result.ErrorCode != "INVALID_ARGUMENTS" || gateway.calls != 0 {
		t.Fatalf("result=%#v err=%v gateway.calls=%d", result, err, gateway.calls)
	}
}

func TestSaturnListReturnsTypedRemoteRosterAlongsideDeliveryReceipts(t *testing.T) {
	wantData := json.RawMessage(`{"room":"lounge","users":["alice","bob"],"count":2,"returnedCount":2,"truncated":false}`)
	executed := verifiedCommandExecution("remote users")
	executed.Data = append(json.RawMessage(nil), wantData...)
	gateway := &runCommandGatewayStub{result: executed}
	caller, _ := api.NewContext("programming", "caller", "", "", false, []string{})
	tool := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "list"), Gateway: gateway}

	result, err := tool.Execute(context.Background(), caller, json.RawMessage(`{"room":"lounge"}`))
	if err != nil || result.IsError || result.DeliveryCount != 1 || result.ActionCount != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	var content struct {
		Messages       []string        `json:"messages"`
		DeliveredCount int             `json:"deliveredCount"`
		ActionCount    int             `json:"actionCount"`
		Data           json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Content), &content); err != nil {
		t.Fatal(err)
	}
	if len(content.Messages) != 1 || content.Messages[0] != "remote users" || content.DeliveredCount != 1 || content.ActionCount != 1 || string(content.Data) != string(wantData) {
		t.Fatalf("content=%s", result.Content)
	}
	descriptor, _ := tool.Descriptor(caller)
	if err := contract.ValidateResult(descriptor.ResultSchema(), []byte(result.Content)); err != nil {
		t.Fatalf("typed result violates saturn_list schema: %v", err)
	}
}

func TestNonListSaturnCommandsDoNotAdvertiseRemoteRosterData(t *testing.T) {
	caller, _ := api.NewContextWithCapabilities("programming", "caller", "", "", false, []string{}, []api.Capability{api.ModerationCommands})
	for _, canonical := range []string{"weather", "kick"} {
		descriptor, err := (agenttool.SaturnCommand{Definition: agentCommandDefinition(t, canonical)}).Descriptor(caller)
		if err != nil {
			t.Fatal(err)
		}
		schema := string(descriptor.ResultSchema())
		if strings.Contains(schema, `"room"`) || strings.Contains(schema, `"users"`) || strings.Contains(schema, `"returnedCount"`) || strings.Contains(schema, `"truncated"`) {
			t.Fatalf("saturn_%s advertises unrelated remote roster data: %s", canonical, schema)
		}
	}
}

func TestSaturnModerationReviewRejectsKickBeforeGateway(t *testing.T) {
	gateway := &runCommandGatewayStub{result: verifiedCommandExecution("not reached")}
	moderator, _ := api.NewContextWithModerationTarget("room", "bot", "creator", "", false, []string{}, []api.Capability{api.ModerationCommands}, "alice")
	tool := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "kick"), Gateway: gateway}
	result, err := tool.Execute(context.Background(), moderator, json.RawMessage(`{"nick":"bob"}`))
	if err != nil || !result.IsError || result.ErrorCode != "TOOL_NOT_AUTHORIZED" || gateway.calls != 0 {
		t.Fatalf("result=%#v calls=%d err=%v", result, gateway.calls, err)
	}
}

func TestModerationReviewRegistryExposesOnlyMuteAndNativeReads(t *testing.T) {
	caller, err := api.NewContextWithModerationTarget(
		"room", "bot", "creator", "", false, []string{"alice"},
		[]api.Capability{api.ModerationCommands, api.PermanentBan, api.AdminCommands}, "alice",
	)
	if err != nil {
		t.Fatal(err)
	}
	tools := []agenttool.Tool{
		agenttool.RoomUsers{},
		agenttool.RunCommand{},
		agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "mute")},
		agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "nuke")},
		agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "notes")},
	}
	allowed := make([]string, len(tools))
	for index, registered := range tools {
		allowed[index] = registered.Name()
	}
	registry := agenttool.NewRegistry(tools, allowed)
	manifest, err := registry.Manifest(caller)
	if err != nil {
		t.Fatal(err)
	}
	visible := map[string]bool{}
	for _, entry := range manifest.Tools {
		visible[entry.Name] = true
	}
	if len(visible) != 2 || !visible["room_users"] || !visible["saturn_mute"] {
		t.Fatalf("review manifest=%#v", visible)
	}
	for _, name := range []string{"run_command", "saturn_nuke", "saturn_notes"} {
		if _, found := registry.Find(caller, name); found {
			t.Errorf("review registry found %q", name)
		}
	}
	if _, found := registry.Find(caller, "saturn_mute"); !found {
		t.Fatal("review registry hid saturn_mute")
	}
}

func TestModerationReviewSaturnDirectExecutionAllowsOnlyMatchingMute(t *testing.T) {
	caller, err := api.NewContextWithModerationTarget(
		"room", "bot", "creator", "", false, []string{"Alice"},
		[]api.Capability{api.ModerationCommands, api.PermanentBan, api.AdminCommands}, "alice",
	)
	if err != nil {
		t.Fatal(err)
	}
	gateway := &runCommandGatewayStub{result: verifiedCommandExecution("done")}
	for _, test := range []struct {
		canonical string
		arguments string
	}{
		{canonical: "nuke", arguments: `{"room":"other"}`},
		{canonical: "notes", arguments: `{"operation":"purge"}`},
		{canonical: "kick", arguments: `{"nick":"alice"}`},
	} {
		command := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, test.canonical), Gateway: gateway}
		result, err := command.Execute(context.Background(), caller, json.RawMessage(test.arguments))
		if err != nil || !result.IsError || result.ErrorCode != "TOOL_NOT_AUTHORIZED" || gateway.calls != 0 {
			t.Fatalf("%s result=%#v err=%v calls=%d", test.canonical, result, err, gateway.calls)
		}
	}

	mute := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "mute"), Gateway: gateway}
	result, err := mute.Execute(context.Background(), caller, json.RawMessage(`{"nick":"@ALICE"}`))
	if err != nil || result.IsError || gateway.calls != 1 || gateway.command != "mute" || gateway.arguments != "@ALICE" {
		t.Fatalf("matching mute result=%#v err=%v gateway=%#v", result, err, gateway)
	}
}

func TestSaturnNukeDirectExecutionRequiresPermanentBanCapability(t *testing.T) {
	gateway := &runCommandGatewayStub{result: verifiedCommandExecution("nuke accepted")}
	command := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "nuke"), Gateway: gateway}
	moderator, err := api.NewContextWithCapabilities("room", "moderator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
	if err != nil {
		t.Fatal(err)
	}
	result, err := command.Execute(context.Background(), moderator, json.RawMessage(`{"room":"other"}`))
	if err != nil || !result.IsError || result.ErrorCode != "TOOL_NOT_AUTHORIZED" || gateway.calls != 0 {
		t.Fatalf("moderator result=%#v err=%v calls=%d", result, err, gateway.calls)
	}

	creator, err := api.NewContextWithCapabilities("room", "creator", "trip", "", false, []string{}, []api.Capability{api.PermanentBan})
	if err != nil {
		t.Fatal(err)
	}
	result, err = command.Execute(context.Background(), creator, json.RawMessage(`{"room":"other"}`))
	if err != nil || result.IsError || gateway.calls != 1 || gateway.command != "nuke" || gateway.arguments != "other" {
		t.Fatalf("creator result=%#v err=%v gateway=%#v", result, err, gateway)
	}
}

func TestSaturnCommandDoesNotTreatUntypedGatewayErrorAsKnownRejection(t *testing.T) {
	gateway := &runCommandGatewayStub{err: errors.New("command validation failed")}
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})
	tool := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "weather"), Gateway: gateway}
	result, err := tool.Execute(context.Background(), caller, json.RawMessage(`{"location":"Chisinau"}`))
	if err != nil || !result.IsError || result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || result.EffectState != contract.EffectUnknown || gateway.calls != 1 {
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
		{name: "missing receipt", execution: commandgateway.Execution{Status: commandgateway.OutcomeSucceeded, EffectsCommitted: true, Action: &commandgateway.ActionReceipt{Count: 1}}, wantCode: "UNVERIFIED_ROOM_DELIVERY"},
		{name: "zero receipt", execution: commandgateway.Execution{Status: commandgateway.OutcomeSucceeded, EffectsCommitted: true, Action: &commandgateway.ActionReceipt{Count: 1}, Delivery: &commandgateway.DeliveryReceipt{}}, wantCode: "UNVERIFIED_ROOM_DELIVERY"},
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

func TestSaturnKickAcceptsVerifiedActionWithoutRoomDelivery(t *testing.T) {
	moderator, _ := api.NewContextWithCapabilities("room", "moderator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
	tool := agenttool.SaturnCommand{
		Definition: agentCommandDefinition(t, "kick"),
		Gateway: &runCommandGatewayStub{result: commandgateway.Execution{
			Status:           commandgateway.OutcomeSucceeded,
			EffectsCommitted: true,
			Action:           &commandgateway.ActionReceipt{Count: 1},
		}},
	}
	descriptor, err := tool.Descriptor(moderator)
	if err != nil || descriptor.ResultMode() != contract.ModelData {
		t.Fatalf("descriptor=%#v err=%v", descriptor, err)
	}

	result, err := tool.Execute(context.Background(), moderator, json.RawMessage(`{"nick":"raider"}`))

	if err != nil || result.IsError || !result.EffectsCommitted || result.VerifiedRoomDelivery() || result.DeliveryCount != 0 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if err := contract.ValidateResult(descriptor.ResultSchema(), []byte(result.Content)); err != nil {
		t.Fatalf("silent kick result violates its declared schema: content=%s err=%v", result.Content, err)
	}
	var content struct {
		ActionCount    int `json:"actionCount"`
		DeliveredCount int `json:"deliveredCount"`
	}
	if json.Unmarshal([]byte(result.Content), &content) != nil || content.ActionCount != 1 || content.DeliveredCount != 0 {
		t.Fatalf("result content=%s", result.Content)
	}
	registry := agenttool.NewRegistry([]agenttool.Tool{tool}, []string{tool.Name()})
	executed := (&execution.Executor{Registry: registry, Ledger: execution.NewLedger(map[string]int{tool.Name(): 1}, 1)}).Execute(
		context.Background(), moderator, execution.Call{ID: "kick-call", Name: tool.Name(), Arguments: json.RawMessage(`{"nick":"raider"}`)},
	)
	if executed.IsError || !executed.EffectsCommitted || executed.DeliveryCount != 0 {
		t.Fatalf("executor rejected verified silent kick: %#v", executed)
	}
}

func TestSaturnKickRejectsUnexpectedArgumentsBeforeGateway(t *testing.T) {
	gateway := &runCommandGatewayStub{result: verifiedCommandExecution("not reached")}
	moderator, _ := api.NewContextWithCapabilities("room", "moderator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
	tool := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "kick"), Gateway: gateway}
	result, err := tool.Execute(context.Background(), moderator, json.RawMessage(`{"nick":"raid","targets":["spam"]}`))
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
		{name: "moderator kick", caller: moderator, canonical: "kick", arguments: `{"nick":"@raider"}`, wantTail: "@raider"},
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
