package command

import (
	"context"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type Unban struct {
	AccessLevel model.Role
	engine      common.Engine
	chatMessage *model.ChatMessage
}

func (u *Unban) GetAliases() []string {
	return []string{"unban"}
}

func (u *Unban) GetRole() *model.Role {
	return &u.AccessLevel
}

func (u *Unban) NewInstance(engine common.Engine, chatMessage *model.ChatMessage) common.Command {
	return &Unban{
		AccessLevel: model.MODERATOR,
		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *Unban) Execute() {
	_, _ = (&simpleUnbanCommand{commandBase: commandBase{engine: u.engine, message: u.chatMessage}}).Execute(context.Background())
}
