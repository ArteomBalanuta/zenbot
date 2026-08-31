package message

import (
	"context"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type dispatchTestEngine struct {
	common.Engine
	commands map[string]common.CommandMetadata
	allowed  bool
	chats    int
}

func (e *dispatchTestEngine) GetPrefix() string { return "!" }
func (e *dispatchTestEngine) GetEnabledCommands() *map[string]common.CommandMetadata {
	return &e.commands
}
func (e *dispatchTestEngine) IsUserAuthorized(*model.User, *model.Role) bool { return e.allowed }
func (e *dispatchTestEngine) SendChatMessage(string, string, bool) (string, error) {
	e.chats++
	return "", nil
}

type dispatchTestCommand struct{ executed *bool }

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
