package tool_test

import (
	"context"
	"encoding/json"
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
	d, err := (agenttool.RunCommand{}).Descriptor(api.Context{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "run_command" || d.Effect() != contract.Action || d.ResultMode() != contract.RoomDelivery || d.Idempotent() || d.Timeout() != 10*time.Second {
		t.Fatalf("descriptor=%#v", d)
	}
	if string(d.Parameters()) != `{"type":"object","additionalProperties":false,"properties":{"command":{"type":"string","enum":["h","help","info","list","p","ping","t","time","users","v","version","w","weather"]},"arguments":{"type":"string","maxLength":4000}},"required":["command"]}` {
		t.Fatalf("parameters=%s", d.Parameters())
	}
	if string(d.ResultSchema()) != `{"type":"object","additionalProperties":false,"properties":{"messages":{"type":"array","items":{"type":"string"}},"deliveredCount":{"type":"integer"}},"required":["messages","deliveredCount"]}` {
		t.Fatalf("result=%s", d.ResultSchema())
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
