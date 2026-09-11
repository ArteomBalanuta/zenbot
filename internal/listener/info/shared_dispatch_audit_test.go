package info

import (
	"context"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type auditWhisperDispatchController struct {
	active, acquired, released int
}

func TestSharedInfoDispatchAuditCancellationBetweenHandlers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	later := false
	chain := NewChain(HandlerFunc(func(context.Context, *Context) (bool, error) { cancel(); return true, nil }), HandlerFunc(func(context.Context, *Context) (bool, error) { later = true; return true, nil }))
	if err := chain.Process(ctx, &model.InfoMessage{}, nil); err != context.Canceled || later {
		t.Fatalf("err=%v later=%t", err, later)
	}
}

type legacyPrivateAuditEngine struct {
	common.Engine
	bodies []string
}

func (*legacyPrivateAuditEngine) GetChannel() string { return "trusted" }
func (e *legacyPrivateAuditEngine) LogMessage(_, _, _, body, _ string) (int64, error) {
	e.bodies = append(e.bodies, body)
	return 1, nil
}

func TestSharedInfoDispatchAuditPrivateBodyNeverUsesPublicLogger(t *testing.T) {
	e := &legacyPrivateAuditEngine{}
	secret := "private token abc"
	_, err := (AuditWhisperCommand{}).Handle(context.Background(), &Context{Engine: e, ChatMessage: &model.ChatMessage{Text: secret, IsWhisper: true}})
	if err == nil || strings.Contains(err.Error(), secret) || len(e.bodies) != 0 {
		t.Fatalf("private audit leaked: err=%v bodies=%v", err, e.bodies)
	}
}

type failingPrivateAuditEngine struct {
	legacyPrivateAuditEngine
	failure error
	ctx     context.Context
	record  model.MessageRecord
}

func (e *failingPrivateAuditEngine) LogMessageRecord(ctx context.Context, record model.MessageRecord) (int64, error) {
	e.ctx = ctx
	e.record = record
	return 0, e.failure
}
func TestSharedInfoDispatchAuditStoreErrorStopsChain(t *testing.T) {
	failure := errors.New("typed store unavailable")
	e := &failingPrivateAuditEngine{failure: failure}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	later := false
	chain := NewChain(HandlerFunc(func(_ context.Context, c *Context) (bool, error) {
		c.ChatMessage = &model.ChatMessage{Text: "private", IsWhisper: true}
		return true, nil
	}), AuditWhisperCommand{}, HandlerFunc(func(context.Context, *Context) (bool, error) { later = true; return true, nil }))
	if err := chain.Process(ctx, &model.InfoMessage{}, e); !errors.Is(err, failure) || later || e.ctx != ctx || e.record.Visibility != "WHISPER" || len(e.bodies) != 0 {
		t.Fatalf("err=%v later=%t record=%+v legacy=%v", err, later, e.record, e.bodies)
	}
}

func (c *auditWhisperDispatchController) BeginDispatch(context.Context) (func(), error) {
	c.active++
	c.acquired++
	return func() { c.active--; c.released++ }, nil
}
func (*auditWhisperDispatchController) RequestRestart(context.Context) error  { return nil }
func (*auditWhisperDispatchController) RequestShutdown(context.Context) error { return nil }

type auditWhisperDispatchEngine struct {
	common.Engine
	controller *auditWhisperDispatchController
	command    *auditWhisperDispatchCommand
	user       *model.User
}

func (*auditWhisperDispatchEngine) GetPrefix() string  { return "!" }
func (*auditWhisperDispatchEngine) GetChannel() string { return "programming" }
func (e *auditWhisperDispatchEngine) HostLifecycleController() common.HostLifecycleController {
	return e.controller
}
func (e *auditWhisperDispatchEngine) GetActiveUsers() *map[*model.User]struct{} {
	users := map[*model.User]struct{}{e.user: {}}
	return &users
}
func (*auditWhisperDispatchEngine) IsUserAuthorized(*model.User, *model.Role) bool { return true }
func (e *auditWhisperDispatchEngine) GetEnabledCommands() *map[string]common.CommandMetadata {
	commands := map[string]common.CommandMetadata{"probe": {Alias: "probe", Command: func(*model.ChatMessage) common.Command { return e.command }}}
	return &commands
}

type auditWhisperDispatchCommand struct {
	controller *auditWhisperDispatchController
	calls      int
	gateHeld   bool
	ctx        context.Context
}

func (c *auditWhisperDispatchCommand) Execute() { c.ExecuteContext(context.Background()) }
func (c *auditWhisperDispatchCommand) ExecuteContext(ctx context.Context) {
	c.calls++
	c.ctx = ctx
	c.gateHeld = c.controller.active > 0
}
func (*auditWhisperDispatchCommand) GetRole() *model.Role { role := model.ADMIN; return &role }
func (*auditWhisperDispatchCommand) GetAliases() []string { return []string{"probe"} }
func (c *auditWhisperDispatchCommand) NewInstance(common.Engine, *model.ChatMessage) common.Command {
	return c
}

func newAuditWhisperDispatch() (*auditWhisperDispatchEngine, *Context) {
	controller := &auditWhisperDispatchController{}
	command := &auditWhisperDispatchCommand{controller: controller}
	engine := &auditWhisperDispatchEngine{controller: controller, command: command, user: &model.User{Name: "alice", Trip: "Trip"}}
	return engine, &Context{Engine: engine, ChatMessage: &model.ChatMessage{Name: "alice", Trip: "Trip", Text: "!probe", IsWhisper: true}}
}

func TestSharedInfoDispatchAuditUsesLifecycleGate(t *testing.T) {
	engine, invocation := newAuditWhisperDispatch()
	_, err := (DispatchWhisperCommand{}).Handle(context.Background(), invocation)
	if err != nil || engine.command.calls != 1 || !engine.command.gateHeld || engine.controller.active != 0 || engine.controller.acquired != 1 || engine.controller.released != 1 {
		t.Fatalf("whisper dispatch bypassed lifecycle gate: command=%+v controller=%+v err=%v", engine.command, engine.controller, err)
	}
}

func TestSharedInfoDispatchAuditPrecancelStopsLegacyCommand(t *testing.T) {
	engine, invocation := newAuditWhisperDispatch()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (DispatchWhisperCommand{}).Handle(ctx, invocation)
	if engine.command.calls != 0 || err != context.Canceled {
		t.Fatalf("canceled whisper still invoked legacy command: calls=%d err=%v", engine.command.calls, err)
	}
}

func TestSharedInfoDispatchAuditWhisperCarriesTrustedRoom(t *testing.T) {
	engine, invocation := newAuditWhisperDispatch()
	invocation.ChatMessage = nil
	invocation.Message = &model.InfoMessage{From: "alice", Channel: "untrusted-payload-room", Text: "alice whispered: !probe"}
	next, err := (ConvertWhisperToChatMessage{}).Handle(context.Background(), invocation)
	if err != nil || !next || invocation.ChatMessage == nil || invocation.ChatMessage.Channel != engine.GetChannel() || !invocation.ChatMessage.Whisper || !invocation.ChatMessage.IsWhisper || invocation.ChatMessage.Type != "whisper" {
		t.Fatalf("converted whisper lost trusted room: next=%t err=%v message=%+v", next, err, invocation.ChatMessage)
	}
}
