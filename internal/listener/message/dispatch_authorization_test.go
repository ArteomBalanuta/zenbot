package message

import (
	"context"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type dispatchTestEngine struct {
	common.Engine
	commands           map[string]common.CommandMetadata
	allowed            bool
	chats              int
	authorizationCalls int
}

func (e *dispatchTestEngine) GetPrefix() string { return "!" }
func (e *dispatchTestEngine) GetEnabledCommands() *map[string]common.CommandMetadata {
	return &e.commands
}
func (e *dispatchTestEngine) IsUserAuthorized(*model.User, *model.Role) bool {
	e.authorizationCalls++
	return e.allowed
}
func (e *dispatchTestEngine) SendChatMessage(string, string, bool) (string, error) {
	e.chats++
	return "", nil
}

type dispatchTestCommand struct{ executed *bool }

type lifecycleAuthorizedDispatchTestCommand struct{ dispatchTestCommand }

func (c *lifecycleAuthorizedDispatchTestCommand) Authorize(*model.User) bool { return true }

type contextualDispatchTestCommand struct {
	executed *bool
	ctx      context.Context
}

func (c *contextualDispatchTestCommand) Execute() { *c.executed = true }
func (c *contextualDispatchTestCommand) ExecuteContext(ctx context.Context) {
	*c.executed = true
	c.ctx = ctx
}
func (c *contextualDispatchTestCommand) GetRole() *model.Role { r := model.MODERATOR; return &r }
func (c *contextualDispatchTestCommand) GetAliases() []string { return []string{"contextual"} }
func (c *contextualDispatchTestCommand) NewInstance(common.Engine, *model.ChatMessage) common.Command {
	return c
}

func (c *dispatchTestCommand) Execute()                                                     { *c.executed = true }
func (c *dispatchTestCommand) GetRole() *model.Role                                         { r := model.MODERATOR; return &r }
func (c *dispatchTestCommand) GetAliases() []string                                         { return []string{"auth"} }
func (c *dispatchTestCommand) NewInstance(common.Engine, *model.ChatMessage) common.Command { return c }

func TestDispatchUserCommandRejectsUnauthorizedPrincipal(t *testing.T) {
	executed := false
	cmd := &dispatchTestCommand{executed: &executed}
	e := &dispatchTestEngine{allowed: false, commands: map[string]common.CommandMetadata{
		"auth": {Alias: "auth", Command: func(*model.ChatMessage) common.Command { return cmd }},
	}}
	next, err := (DispatchUserCommand{}).Handle(context.Background(), &Context{
		Engine: e, Message: &model.ChatMessage{Name: "alice", Text: "!auth"}, Author: &model.User{Name: "alice"},
	})
	if err != nil || next || executed || e.chats != 1 {
		t.Fatalf("next=%v err=%v executed=%v chats=%d", next, err, executed, e.chats)
	}
}

func TestDispatchUserCommandAuthorizesBeforeContextualExecution(t *testing.T) {
	caller, cancel := context.WithCancel(context.Background())
	defer cancel()
	executed := false
	cmd := &contextualDispatchTestCommand{executed: &executed}
	e := &dispatchTestEngine{allowed: true, commands: map[string]common.CommandMetadata{
		"contextual": {Alias: "contextual", Command: func(*model.ChatMessage) common.Command { return cmd }},
	}}

	next, err := (DispatchUserCommand{}).Handle(caller, &Context{
		Engine: e, Message: &model.ChatMessage{Name: "alice", Text: "!contextual"}, Author: &model.User{Name: "alice"},
	})

	if err != nil || next || !executed || cmd.ctx != caller || e.chats != 0 || e.authorizationCalls != 1 {
		t.Fatalf("next=%v err=%v executed=%v context=%#v chats=%d authorizationCalls=%d", next, err, executed, cmd.ctx, e.chats, e.authorizationCalls)
	}
}

func TestDispatchUserCommandLeavesNonContextCommandExecutionUnchanged(t *testing.T) {
	executed := false
	cmd := &dispatchTestCommand{executed: &executed}
	e := &dispatchTestEngine{allowed: true, commands: map[string]common.CommandMetadata{
		"auth": {Alias: "auth", Command: func(*model.ChatMessage) common.Command { return cmd }},
	}}

	next, err := (DispatchUserCommand{}).Handle(context.Background(), &Context{
		Engine: e, Message: &model.ChatMessage{Name: "alice", Text: "!auth"}, Author: &model.User{Name: "alice"},
	})

	if err != nil || next || !executed || e.chats != 0 {
		t.Fatalf("next=%v err=%v executed=%v chats=%d", next, err, executed, e.chats)
	}
}

func TestDispatchUserCommandUsesOptionalLifecycleAuthorizer(t *testing.T) {
	executed := false
	cmd := &lifecycleAuthorizedDispatchTestCommand{dispatchTestCommand: dispatchTestCommand{executed: &executed}}
	e := &dispatchTestEngine{allowed: false, commands: map[string]common.CommandMetadata{
		"lifecycle": {Alias: "lifecycle", Command: func(*model.ChatMessage) common.Command { return cmd }},
	}}

	_, err := (DispatchUserCommand{}).Handle(context.Background(), &Context{
		Engine: e, Message: &model.ChatMessage{Name: "alice", Text: "!lifecycle"}, Author: &model.User{Name: "alice"},
	})
	if err != nil || !executed || e.chats != 0 || e.authorizationCalls != 0 {
		t.Fatalf("err=%v executed=%v chats=%d authorizationCalls=%d", err, executed, e.chats, e.authorizationCalls)
	}
}
