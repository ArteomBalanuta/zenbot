package command

import (
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type Unlock struct {
	AccessLevel model.Role
	engine      common.Engine
	chatMessage *model.ChatMessage
}

func (u *Unlock) GetAliases() []string {
	return []string{"unlock", "unlockroom", "unlockchannel", "unr"}
}

func (u *Unlock) GetRole() *model.Role {
	return &u.AccessLevel
}

func (u *Unlock) NewInstance(engine common.Engine, chatMessage *model.ChatMessage) common.Command {
	return &Unlock{
		AccessLevel: model.MODERATOR,
		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *Unlock) Execute() {
	u.engine.Unlock()
	u.engine.SendChatMessage(u.chatMessage.Name, " room unlocked", u.chatMessage.IsWhisper)
}
