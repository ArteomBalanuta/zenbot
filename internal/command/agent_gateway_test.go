package command

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/common"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/service"
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

type blockingAuditSnapshotGatewayEngine struct {
	*snapshotGatewayEngine
	auditStarted chan struct{}
	releaseAudit chan struct{}
}

func (e *blockingAuditSnapshotGatewayEngine) LogCommand(context.Context, model.CommandAuditRecord) (int64, error) {
	close(e.auditStarted)
	<-e.releaseAudit
	return 1, nil
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
	if err != nil || result.Status != commandgateway.OutcomeSucceeded || !result.EffectsCommitted || result.Delivery == nil || result.Delivery.Count != 1 || e.sends != 1 || len(result.Messages) != 1 {
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
		wantStatus    commandgateway.OutcomeStatus
		wantError     bool
	}{
		{"unknown", "totally_unknown", true, nil, commandgateway.OutcomeRejected, true},
		{"denied", "captcha", false, nil, commandgateway.OutcomeRejected, true},
		{"send", "ping", true, errors.New("send failed"), commandgateway.OutcomeUnknown, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller"}}}, authorized: tc.authorized, sendErr: tc.sendErr}
			result, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, tc.command, "")
			if (err != nil) != tc.wantError || result.Status != tc.wantStatus || result.EffectsCommitted || result.Delivery != nil || len(result.Messages) != 0 {
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
	if err == nil || result.Status == commandgateway.OutcomeSucceeded || result.EffectsCommitted || result.Delivery != nil || e.sends != 0 {
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
	if err != nil || result.Status != commandgateway.OutcomeSucceeded || !result.EffectsCommitted || result.Delivery == nil || result.Delivery.Count != 1 || len(e.raws) != 1 || e.raws[0] != `{"cmd":"enablecaptcha"}` || len(result.Messages) != 1 {
		t.Fatalf("result=%#v raws=%#v messages=%#v err=%v", result, e.raws, result.Messages, err)
	}
}

func TestAgentCommandGatewayRetainsDeliveredExplanationOnRejectedCommand(t *testing.T) {
	e := &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{}}}
	caller, _ := api.NewContextWithCapabilities("room", "moderator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
	result, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, "captcha", "invalid")
	if err != nil || result.Status != commandgateway.OutcomeRejected || !result.EffectsCommitted || result.Delivery == nil || result.Delivery.Count != 1 || len(result.Messages) != 1 {
		t.Fatalf("rejected command lost its delivered explanation: result=%#v err=%v", result, err)
	}
	if len(e.raws) != 0 {
		t.Fatalf("rejected captcha changed room state: %#v", e.raws)
	}
}

type panicAuditGatewayEngine struct{ *gatewayEngine }

func (*panicAuditGatewayEngine) LogCommand(context.Context, model.CommandAuditRecord) (int64, error) {
	panic("audit unavailable")
}

type prefixReceiptEngine struct {
	*gatewayEngine
	prefix string
}

func (e *prefixReceiptEngine) UpdatePrefix(next string) (string, error) {
	previous := e.prefix
	e.prefix = next
	return previous, nil
}

func TestAgentCommandGatewayRetainsPrefixMutationWhenReplyFails(t *testing.T) {
	e := &prefixReceiptEngine{gatewayEngine: &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{}}, sendErr: errors.New("delivery failed")}, prefix: "!"}
	caller, _ := api.NewContextWithCapabilities("room", "admin", "trip", "", false, []string{}, []api.Capability{api.AdminCommands})
	result, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, "prefix", "$prefix")
	if err != nil || e.prefix != "$prefix" || !result.EffectsCommitted || result.Action == nil || result.Action.Count != 1 || result.Delivery != nil {
		t.Fatalf("prefix mutation lost its receipt after failed reply: prefix=%q result=%#v err=%v", e.prefix, result, err)
	}
}

func TestAgentCommandGatewayRetainsDeliveryWhenLaterCodePanics(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("gateway panic discarded committed delivery: %v", recovered)
		}
	}()
	e := &panicAuditGatewayEngine{gatewayEngine: &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller"}}}, authorized: true}}
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})
	result, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, "ping", "")
	if err != nil || result.Status != commandgateway.OutcomeUnknown || !result.EffectsCommitted || result.Delivery == nil || result.Delivery.Count != 1 {
		t.Fatalf("panic outcome lost its receipt: result=%#v err=%v", result, err)
	}
}

func TestAgentCommandGatewayKickReturnsNotFoundWithoutSendingForAbsentActiveTarget(t *testing.T) {
	e := &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{}}, authorized: true}
	caller, err := api.NewContextWithCapabilities("room", "moderator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
	if err != nil {
		t.Fatal(err)
	}

	result, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, "kick", "absent")

	if err != nil || result.Status != commandgateway.OutcomeNotFound || result.EffectsCommitted || result.Delivery != nil || len(e.raws) != 0 || len(e.chats) != 0 {
		t.Fatalf("result=%#v raws=%#v chats=%#v err=%v", result, e.raws, e.chats, err)
	}
}

func TestAgentCommandGatewayKickVerifiesSilentActiveTargetAction(t *testing.T) {
	e := &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{"Raider": {Name: "Raider"}}}, authorized: true}
	caller, err := api.NewContextWithCapabilities("room", "moderator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
	if err != nil {
		t.Fatal(err)
	}

	result, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, "kick", "@raider")

	if err != nil || result.Status != commandgateway.OutcomeSucceeded || !result.EffectsCommitted || result.Action == nil || result.Action.Count != 1 || result.Delivery != nil || len(result.Messages) != 0 || len(e.raws) != 1 || e.raws[0] != `{"cmd":"kick","nick":"Raider"}` || len(e.chats) != 0 {
		t.Fatalf("result=%#v raws=%#v chats=%#v err=%v", result, e.raws, e.chats, err)
	}
}

func TestAgentCommandGatewayWaitsForEverySnapshotBackedCommandOutcome(t *testing.T) {
	public, _ := api.NewContext("programming", "caller", "trip", "hash", false, []string{})
	moderator, _ := api.NewContextWithCapabilities("programming", "caller", "trip", "hash", false, []string{}, []api.Capability{api.ModerationCommands})
	creator, _ := api.NewContextWithCapabilities("programming", "caller", "trip", "hash", false, []string{}, []api.Capability{api.PermanentBan})
	tests := []struct {
		name      string
		command   string
		arguments string
		caller    api.Context
	}{
		{name: "remote list", command: "list", arguments: "lounge", caller: public},
		{name: "remote message", command: "msgchannel", arguments: "lounge hello", caller: public},
		{name: "remote nuke", command: "nuke", arguments: "lounge", caller: creator},
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
				if execution.err != nil || execution.result.Status != commandgateway.OutcomeSucceeded || !execution.result.EffectsCommitted || execution.result.Delivery == nil || execution.result.Delivery.Count != 1 || len(execution.result.Messages) != 1 || execution.result.Messages[0] != "remote operation result" {
					t.Fatalf("execution=%+v", execution)
				}
			case <-time.After(time.Second):
				t.Fatal("gateway did not resume after snapshot completion")
			}
		})
	}
}

func TestAgentCommandGatewayPreservesTypedRemoteListDataAndDeliveryReceipts(t *testing.T) {
	engine := &blockingAuditSnapshotGatewayEngine{
		snapshotGatewayEngine: &snapshotGatewayEngine{
			gatewayEngine: &gatewayEngine{
				commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller"}}},
				authorized:        true,
			},
			requests: make(chan snapshot.RoomSnapshotRequest, 1),
		},
		auditStarted: make(chan struct{}),
		releaseAudit: make(chan struct{}),
	}
	caller, _ := api.NewContext("programming", "caller", "", "", false, []string{})
	completed := make(chan struct {
		result CommandExecution
		err    error
	}, 1)
	go func() {
		result, err := NewAgentCommandGateway(engine).Execute(context.Background(), caller, "list", "lounge")
		completed <- struct {
			result CommandExecution
			err    error
		}{result: result, err: err}
	}()

	request := <-engine.requests
	<-engine.auditStarted
	operationResult, err := request.Operation.Apply(snapshot.RoomSnapshotContext{TargetChannel: request.TargetChannel}, snapshot.Snapshot{Users: []*model.User{
		{Name: "bob", Hash: "b"},
		{Name: "alice", Hash: "a", Trip: "known"},
		{Name: "duplicate", Hash: "other", Trip: "known"},
		nil,
	}})
	if err != nil {
		t.Fatal(err)
	}
	wantData := append(json.RawMessage(nil), operationResult.Data...)
	request.OnComplete(operationResult)
	for index := range operationResult.Data {
		operationResult.Data[index] = 'x'
	}
	close(engine.releaseAudit)

	select {
	case execution := <-completed:
		if execution.err != nil || execution.result.Status != commandgateway.OutcomeSucceeded || !execution.result.EffectsCommitted || execution.result.Action == nil || execution.result.Action.Count != 1 || execution.result.Delivery == nil || execution.result.Delivery.Count != 1 || len(execution.result.Messages) != 1 {
			t.Fatalf("execution=%+v", execution)
		}
		if got := commandExecutionData(t, execution.result); !reflect.DeepEqual(got, wantData) {
			t.Fatalf("data=%s, want %s", got, wantData)
		}
	case <-time.After(time.Second):
		t.Fatal("gateway did not resume after typed snapshot completion")
	}
}

func commandExecutionData(t *testing.T, execution CommandExecution) json.RawMessage {
	t.Helper()
	field := reflect.ValueOf(execution).FieldByName("Data")
	if !field.IsValid() {
		t.Fatal("commandgateway.Execution.Data is missing")
	}
	data, ok := field.Interface().(json.RawMessage)
	if !ok {
		t.Fatalf("commandgateway.Execution.Data has type %s, want json.RawMessage", field.Type())
	}
	return append(json.RawMessage(nil), data...)
}

func TestAgentCommandGatewayDoesNotVerifyLegacySuccessWithoutDelivery(t *testing.T) {
	e := &gatewayEngine{
		commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller"}}},
		authorized:        true,
		sendErr:           errors.New("delivery failed"),
	}
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})

	result, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, "say", "hello")

	if err != nil {
		t.Fatal(err)
	}
	if result.Status != commandgateway.OutcomeSucceeded || result.EffectsCommitted || result.Action != nil {
		t.Fatalf("terminal command outcome=%#v", result)
	}
	if result.Delivery != nil || len(result.Messages) != 0 {
		t.Fatalf("failed send was promoted to verified delivery: %#v", result)
	}
}

func TestAgentCommandGatewayMapsMissingRecordToNotFoundOutcome(t *testing.T) {
	e := &gatewayEngine{
		commandEngineStub: commandEngineStub{
			users:  map[string]*model.User{"caller": {Name: "caller"}},
			bundle: &service.Bundle{Users: &service.UserService{Queries: &lastOnlineCommandQueriesStub{}}},
		},
		authorized: true,
	}
	caller, _ := api.NewContext("room", "caller", "", "", false, []string{})

	result, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, "lastonline", "absent")

	if err != nil || result.Status != commandgateway.OutcomeNotFound || result.EffectsCommitted || result.Delivery != nil {
		t.Fatalf("result=%#v err=%v", result, err)
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
	completed := make(chan struct {
		result CommandExecution
		err    error
	}, 1)
	go func() {
		result, err := NewAgentCommandGateway(engine).Execute(context.Background(), caller, "list", "lounge")
		completed <- struct {
			result CommandExecution
			err    error
		}{result: result, err: err}
	}()
	request := <-engine.requests
	request.OnComplete(snapshot.Failed("remote room failed"))

	select {
	case outcome := <-completed:
		if outcome.err != nil || outcome.result.Status != commandgateway.OutcomeUnknown || outcome.result.EffectsCommitted || outcome.result.Delivery != nil {
			t.Fatalf("outcome=%#v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("gateway did not resume after snapshot failure")
	}
}
