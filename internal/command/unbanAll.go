package command

import (
	"context"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type UnbanAll struct {
	AccessLevel model.Role
	engine      common.Engine
	chatMessage *model.ChatMessage
}

func (u *UnbanAll) GetAliases() []string {
	return []string{"unbanall", "pardonall"}
}

func (u *UnbanAll) GetRole() *model.Role {
	return &u.AccessLevel
}

func (u *UnbanAll) NewInstance(engine common.Engine, chatMessage *model.ChatMessage) common.Command {
	return &UnbanAll{
		AccessLevel: model.MODERATOR,
		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *UnbanAll) Execute() {
	_, _ = (&simpleUnbanAllCommand{commandBase: commandBase{engine: u.engine, message: u.chatMessage}}).Execute(context.Background())
}
