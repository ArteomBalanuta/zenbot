package command

import (
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type Lock struct {
	AccessLevel model.Role
	engine      common.Engine
	chatMessage *model.ChatMessage
}

func (u *Lock) GetAliases() []string {
	return []string{"lock", "lockroom", "lockchannel"}
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
	u.engine.Lock()
	u.engine.SendChatMessage(u.chatMessage.Name, " room locked", u.chatMessage.IsWhisper)
}
