package command

import (
	"context"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type Ban struct {
	AccessLevel model.Role
	engine      common.Engine
	chatMessage *model.ChatMessage
}

func (u *Ban) GetAliases() []string {
	return []string{"ban"}
}

func (u *Ban) GetRole() *model.Role {
	return &u.AccessLevel
}

func (u *Ban) NewInstance(engine common.Engine, chatMessage *model.ChatMessage) common.Command {
	return &Ban{
		AccessLevel: model.MODERATOR,
		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *Ban) Execute() {
	_, _ = (&simpleBanCommand{commandBase: commandBase{engine: u.engine, message: u.chatMessage}}).Execute(context.Background())
}
