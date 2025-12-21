package command

import (
	"fmt"
	"strings"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type Unban struct {
	AccessLevel model.Role
	engine      common.Engine
	chatMessage *model.ChatMessage
}

func (u *Unban) GetAliases() []string {
	return []string{"unban", "ub"}
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
	hash := u.chatMessage.GetArguments()[1:][0]
	if strings.TrimSpace(hash) != "" {
		u.engine.Unban(hash)
		u.engine.SendChatMessage(u.chatMessage.Name, fmt.Sprintf(" user with hash: %s, unbanned", hash), u.chatMessage.IsWhisper)
	} else {
		u.engine.SendChatMessage(u.chatMessage.Name, " user not found", u.chatMessage.IsWhisper)
	}
}
