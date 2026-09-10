package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/common"
	"zenbot/internal/listener/snapshot"
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

type snapshotGatewayEngine struct {
	*gatewayEngine
	requests chan snapshot.RoomSnapshotRequest
}

func (e *snapshotGatewayEngine) SubmitRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	e.requests <- request
	return nil
}

func (e *snapshotGatewayEngine) SubmitCredentialedRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	e.requests <- request
	return nil
}

func (e *snapshotGatewayEngine) MoveFromServingRoom(context.Context, string, common.NickTarget, common.Channel) (bool, error) {
	return false, nil
}

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
		{"unknown", "totally_unknown", true, nil}, {"denied", "captcha", false, nil}, {"send", "ping", true, errors.New("send failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller"}}}, authorized: tc.authorized, sendErr: tc.sendErr}
			result, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, tc.command, "")
			if err == nil || result.Executed || len(result.Messages) != 0 {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if tc.command != "ping" && e.sends != 0 {
				t.Fatalf("rejected command sent=%d", e.sends)
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

func TestAgentCommandGatewayPreservesTypedModerationOperationsForSyntheticCaller(t *testing.T) {
	e := &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{}}}
	caller, err := api.NewContextWithCapabilities("room", "bot", "creator", "", false, []string{}, []api.Capability{api.ModerationCommands})
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, "captcha", "on")
	if err != nil || !result.Executed || len(e.raws) != 1 || e.raws[0] != `{"cmd":"enablecaptcha"}` || len(result.Messages) != 1 {
		t.Fatalf("result=%#v raws=%#v messages=%#v err=%v", result, e.raws, result.Messages, err)
	}
}

func TestAgentCommandGatewayWaitsForEverySnapshotBackedCommandOutcome(t *testing.T) {
	public, _ := api.NewContext("programming", "caller", "trip", "hash", false, []string{})
	moderator, _ := api.NewContextWithCapabilities("programming", "caller", "trip", "hash", false, []string{}, []api.Capability{api.ModerationCommands})
	tests := []struct {
		name      string
		command   string
		arguments string
		caller    api.Context
	}{
		{name: "remote list", command: "list", arguments: "lounge", caller: public},
		{name: "remote message", command: "msgchannel", arguments: "lounge hello", caller: public},
		{name: "remote nuke", command: "nuke", arguments: "lounge", caller: moderator},
		{name: "remote resurrect fallback", command: "resurrect", arguments: "target lounge programming", caller: moderator},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := &snapshotGatewayEngine{
				gatewayEngine: &gatewayEngine{
					commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller", Trip: "trip", Hash: "hash"}}},
					authorized:        true,
				},
				requests: make(chan snapshot.RoomSnapshotRequest, 1),
			}
			type executionResult struct {
				result CommandExecution
				err    error
			}
			completed := make(chan executionResult, 1)
			go func() {
				result, err := NewAgentCommandGateway(engine).Execute(context.Background(), test.caller, test.command, test.arguments)
				completed <- executionResult{result: result, err: err}
			}()

			var request snapshot.RoomSnapshotRequest
			select {
			case request = <-engine.requests:
			case <-time.After(time.Second):
				t.Fatal("snapshot request was not submitted")
			}
			select {
			case result := <-completed:
				t.Fatalf("gateway returned before snapshot completion: %+v", result)
			default:
			}
			if request.OnComplete == nil {
				t.Fatal("snapshot request has no agent completion observer")
			}
			request.OnComplete(snapshot.Success("remote operation result"))

			select {
			case execution := <-completed:
				if execution.err != nil || !execution.result.Executed || len(execution.result.Messages) != 1 || execution.result.Messages[0] != "remote operation result" {
					t.Fatalf("execution=%+v", execution)
				}
			case <-time.After(time.Second):
				t.Fatal("gateway did not resume after snapshot completion")
			}
		})
	}
}

func TestAgentCommandGatewayReturnsSnapshotFailureInsteadOfEarlySuccess(t *testing.T) {
	engine := &snapshotGatewayEngine{
		gatewayEngine: &gatewayEngine{
			commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller"}}},
			authorized:        true,
		},
		requests: make(chan snapshot.RoomSnapshotRequest, 1),
	}
	caller, _ := api.NewContext("programming", "caller", "", "", false, []string{})
	completed := make(chan error, 1)
	go func() {
		_, err := NewAgentCommandGateway(engine).Execute(context.Background(), caller, "list", "lounge")
		completed <- err
	}()
	request := <-engine.requests
	request.OnComplete(snapshot.Failed("remote room failed"))

	select {
	case err := <-completed:
		if err == nil || err.Error() != "remote room failed" {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("gateway did not resume after snapshot failure")
	}
}
