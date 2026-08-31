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
	arguments := u.chatMessage.GetArguments()
	if len(arguments) < 2 {
		u.engine.SendChatMessage(u.chatMessage.Name, "Example: "+u.engine.GetPrefix()+"unban HjkUEWNlIRH35Xk", u.chatMessage.IsWhisper)
		return
	}
	hash := arguments[1]
	if strings.TrimSpace(hash) != "" {
		u.engine.Unban(hash)
		u.engine.SendChatMessage(u.chatMessage.Name, fmt.Sprintf("%s has been unbanned", hash), u.chatMessage.IsWhisper)
	} else {
		u.engine.SendChatMessage(u.chatMessage.Name, " user not found", u.chatMessage.IsWhisper)
	}
}
