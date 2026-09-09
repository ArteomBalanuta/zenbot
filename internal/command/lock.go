package command

import (
	"context"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type Lock struct {
	AccessLevel model.Role
	engine      common.Engine
	chatMessage *model.ChatMessage
}

func (u *Lock) GetAliases() []string {
	return []string{"lock", "lockroom"}
}

func (u *Lock) GetRole() *model.Role {
	return &u.AccessLevel
}

func (u *Lock) NewInstance(engine common.Engine, chatMessage *model.ChatMessage) common.Command {
	return &Lock{
		AccessLevel: model.MODERATOR,
		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *Lock) Execute() {
	_, _ = (&simpleLockCommand{commandBase: commandBase{engine: u.engine, message: u.chatMessage}}).Execute(context.Background())
}
