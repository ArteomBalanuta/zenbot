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
	"zenbot/internal/agent/commandgateway"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/command"
)

func verifiedCommandExecution(messages ...string) commandgateway.Execution {
	return commandgateway.Execution{
		Status:           commandgateway.OutcomeSucceeded,
		EffectsCommitted: true,
		Action:           &commandgateway.ActionReceipt{Count: 1},
		Messages:         append([]string(nil), messages...),
		Delivery:         &commandgateway.DeliveryReceipt{Count: len(messages)},
	}
}

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
	if string(d.ResultSchema()) != `{"type":"object","additionalProperties":false,"properties":{"messages":{"type":"array","items":{"type":"string"}},"deliveredCount":{"type":"integer"},"actionCount":{"type":"integer"},"data":{"type":"any"}},"required":["messages","deliveredCount","actionCount"]}` {
		t.Fatalf("result=%s", d.ResultSchema())
	}
	if err := contract.ValidateResult(d.ResultSchema(), json.RawMessage(`{"messages":[],"deliveredCount":0,"actionCount":0}`)); err != nil {
		t.Fatalf("optional typed data broke commands without domain data: %v", err)
	}
}

func TestRunCommandDescriptorAddsModerationWithoutDuplicatingDedicatedKickSurface(t *testing.T) {
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
	if json.Unmarshal(d.Parameters(), &schema) != nil || !slices.Contains(schema.Properties["command"].Enum, "mute") || slices.Contains(schema.Properties["command"].Enum, "kick") || slices.Contains(schema.Properties["command"].Enum, "k") || slices.Contains(schema.Properties["command"].Enum, "out") || slices.Contains(schema.Properties["command"].Enum, "ban") {
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
	gateway := &runCommandGatewayStub{result: verifiedCommandExecution("forecast")}
	result, err := (agenttool.RunCommand{Gateway: gateway}).Execute(context.Background(), caller, json.RawMessage(`{"command":" W ","arguments":" Tokyo "}`))
	var body struct {
		Messages       []string `json:"messages"`
		DeliveredCount int      `json:"deliveredCount"`
		ActionCount    int      `json:"actionCount"`
	}
	if json.Unmarshal([]byte(result.Content), &body) != nil || len(body.Messages) != 1 || body.Messages[0] != "forecast" || body.DeliveredCount != 1 || body.ActionCount != 1 {
		t.Fatalf("result content=%s", result.Content)
	}
	if err != nil || result.IsError || gateway.calls != 1 || gateway.command != "w" || gateway.arguments != "Tokyo" {
		t.Fatalf("result=%#v err=%v gateway=%#v", result, err, gateway)
	}
	gateway = &runCommandGatewayStub{result: command.CommandExecution{Status: commandgateway.OutcomeRejected}}
	result, err = (agenttool.RunCommand{Gateway: gateway}).Execute(context.Background(), caller, json.RawMessage(`{"command":"ping"}`))
	if err != nil || !result.IsError || result.ErrorCode != "COMMAND_REJECTED" || gateway.calls != 1 {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, gateway.calls)
	}
}

func TestRunCommandReturnsTypedRemoteListDataAlongsideReceipts(t *testing.T) {
	caller, _ := api.NewContext("programming", "caller", "", "", false, []string{})
	wantData := json.RawMessage(`{"room":"lounge","users":["alice","bob"],"count":2,"returnedCount":2,"truncated":false}`)
	executed := verifiedCommandExecution("remote users")
	executed.Data = append(json.RawMessage(nil), wantData...)
	tool := agenttool.RunCommand{Gateway: &runCommandGatewayStub{result: executed}}
	result, err := tool.Execute(context.Background(), caller, json.RawMessage(`{"command":"list","arguments":"lounge"}`))
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
	if !slices.Equal(content.Messages, []string{"remote users"}) || content.DeliveredCount != 1 || content.ActionCount != 1 || !slices.Equal(content.Data, wantData) {
		t.Fatalf("content=%s", result.Content)
	}
	descriptor, _ := tool.Descriptor(caller)
	if err := contract.ValidateResult(descriptor.ResultSchema(), []byte(result.Content)); err != nil {
		t.Fatalf("typed result violates run_command schema: %v", err)
	}
}

func TestRunCommandDoesNotTreatUntypedGatewayErrorAsKnownRejection(t *testing.T) {
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})
	gateway := &runCommandGatewayStub{err: errors.New("command validation failed")}
	result, err := (agenttool.RunCommand{Gateway: gateway}).Execute(context.Background(), caller, json.RawMessage(`{"command":"ping"}`))
	if err != nil || !result.IsError || result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || result.EffectState != contract.EffectUnknown || gateway.calls != 1 {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, gateway.calls)
	}
}

func TestRunCommandPreservesPartialGatewayReceiptsOnFailure(t *testing.T) {
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})
	for _, status := range []commandgateway.OutcomeStatus{commandgateway.OutcomeRejected, commandgateway.OutcomeUnknown} {
		executed := verifiedCommandExecution("first output delivered")
		executed.Status = status
		gateway := &runCommandGatewayStub{result: executed}
		result, err := (agenttool.RunCommand{Gateway: gateway}).Execute(context.Background(), caller, json.RawMessage(`{"command":"ping"}`))
		if err != nil || !result.IsError || !result.EffectsCommitted || result.DeliveryCount != 1 || result.VerifiedRoomDelivery() {
			t.Fatalf("status=%s result=%#v err=%v", status, result, err)
		}
	}
}

func TestRunCommandPreservesCommittedReceiptAlongsideGatewayError(t *testing.T) {
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})
	gateway := &runCommandGatewayStub{result: verifiedCommandExecution("delivered"), err: errors.New("connection interrupted")}
	result, err := (agenttool.RunCommand{Gateway: gateway}).Execute(context.Background(), caller, json.RawMessage(`{"command":"ping"}`))
	if err != nil || result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || !result.EffectsCommitted || result.DeliveryCount != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestRunCommandModerationReviewRejectsLegacyAliasBeforeGateway(t *testing.T) {
	moderator, _ := api.NewContextWithModerationTarget("room", "bot", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands}, "alice")
	gateway := &runCommandGatewayStub{result: verifiedCommandExecution("moderation complete")}
	result, err := (agenttool.RunCommand{Gateway: gateway}).Execute(context.Background(), moderator, json.RawMessage(`{"command":"mute","arguments":"bob"}`))
	if err != nil || !result.IsError || result.ErrorCode != "TOOL_NOT_AUTHORIZED" || gateway.calls != 0 {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, gateway.calls)
	}
}

func TestRunCommandModerationReviewHasNoLegacyAuthority(t *testing.T) {
	caller, err := api.NewContextWithModerationTarget(
		"room", "bot", "creator", "", false, []string{"alice"},
		[]api.Capability{api.ModerationCommands, api.PermanentBan, api.AdminCommands}, "alice",
	)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := (agenttool.RunCommand{}).Descriptor(caller)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(descriptor.Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	if aliases := schema.Properties["command"].Enum; !slices.Equal(aliases, []string{"dumb", "mute"}) {
		t.Fatalf("review run_command aliases=%#v", aliases)
	}

	gateway := &runCommandGatewayStub{result: verifiedCommandExecution("muted")}
	result, err := (agenttool.RunCommand{Gateway: gateway}).Execute(context.Background(), caller, json.RawMessage(`{"command":"mute","arguments":"alice"}`))
	if err != nil || !result.IsError || result.ErrorCode != "TOOL_NOT_AUTHORIZED" || gateway.calls != 0 {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, gateway.calls)
	}
}

func TestRunCommandRejectsDedicatedKickAliasesBeforeGateway(t *testing.T) {
	moderator, _ := api.NewContextWithCapabilities("room", "moderator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
	gateway := &runCommandGatewayStub{result: commandgateway.Execution{
		Status:           commandgateway.OutcomeSucceeded,
		EffectsCommitted: true,
		Action:           &commandgateway.ActionReceipt{Count: 1},
	}}

	for _, alias := range []string{"kick", "k", "out"} {
		result, err := (agenttool.RunCommand{Gateway: gateway}).Execute(context.Background(), moderator, json.RawMessage(`{"command":"`+alias+`","arguments":"raider"}`))
		if err != nil || !result.IsError || result.ErrorCode != "COMMAND_REJECTED" || gateway.calls != 0 {
			t.Fatalf("alias=%q result=%#v err=%v gateway=%#v", alias, result, err, gateway)
		}
	}
}
