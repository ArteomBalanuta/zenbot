package command

import (
	"strings"
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
	arguments := u.chatMessage.GetArguments()
	if len(arguments) < 2 || (arguments[1] != "on" && arguments[1] != "off") {
		u.engine.SendChatMessage(u.chatMessage.Name, u.engine.GetPrefix()+"lock [on|off]", u.chatMessage.IsWhisper)
		return
	}
	if strings.EqualFold(arguments[1], "on") {
		u.engine.Lock()
		u.engine.SendChatMessage(u.chatMessage.Name, " Room locked!", u.chatMessage.IsWhisper)
		return
	}
	u.engine.Unlock()
	u.engine.SendChatMessage(u.chatMessage.Name, " Room unlocked!", u.chatMessage.IsWhisper)
}
