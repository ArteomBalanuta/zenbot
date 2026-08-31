package command

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/model"
)

type gatewayEngine struct {
	commandEngineStub
	authorized bool
	sendErr    error
	sends      int
}

func (e *gatewayEngine) SendChatMessage(author, text string, whisper bool) (string, error) {
	e.sends++
	if e.sendErr != nil {
		return "", e.sendErr
	}
	return e.commandEngineStub.SendChatMessage(author, text, whisper)
}
func (e *gatewayEngine) IsUserAuthorized(_ *model.User, _ *model.Role) bool { return e.authorized }

func TestAgentCommandGatewayExecutesTrustedPublicCommandAndCapturesSuccessfulSend(t *testing.T) {
	e := &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller", Trip: "trip", Hash: "hash"}}}, authorized: true}
	caller, err := api.NewContext("trusted-room", "caller", "trip", "hash", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, " P ", " Tokyo ")
	if err != nil || !result.Executed || e.sends != 1 || len(result.Messages) != 1 {
		t.Fatalf("result=%#v err=%v sends=%d", result, err, e.sends)
	}
	if e.chats[0][:7] != "caller|" {
		t.Fatalf("synthetic identity was not trusted: %q", e.chats[0])
	}
}

func TestAgentCommandGatewayRejectsUnauthorizedUnknownAndSendFailure(t *testing.T) {
	caller, _ := api.NewContext("room", "caller", "trip", "hash", false, []string{})
	for _, tc := range []struct {
		name, command string
		authorized    bool
		sendErr       error
	}{
		{"unknown", "whois", true, nil}, {"denied", "ping", false, nil}, {"send", "ping", true, errors.New("send failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller"}}}, authorized: tc.authorized, sendErr: tc.sendErr}
			result, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, tc.command, "")
			if err == nil || result.Executed || len(result.Messages) != 0 {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if tc.command != "ping" && e.sends != 0 {
				t.Fatalf("unknown command sent=%d", e.sends)
			}
		})
	}
}

func TestAgentCommandGatewayDoesNotExecuteCancelledContext(t *testing.T) {
	e := &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller"}}}, authorized: true}
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := NewAgentCommandGateway(e).Execute(ctx, caller, "ping", "")
	if err == nil || result.Executed || e.sends != 0 {
		t.Fatalf("result=%#v err=%v sends=%d", result, err, e.sends)
	}
}
