package command

import (
	"strings"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type Afk struct {
	AccessLevel model.Role
	engine      common.Engine
	chatMessage *model.ChatMessage
}

func (u *Afk) GetAliases() []string {
	return []string{"afk", "a"}
}

func (u *Afk) GetRole() *model.Role {
	return &u.AccessLevel
}

func (u *Afk) NewInstance(engine common.Engine, chatMessage *model.ChatMessage) common.Command {
	return &Afk{
		AccessLevel: model.USER,
		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *Afk) Execute() {
	if u.chatMessage.Trip == "" {
		_, _ = u.engine.SendChatMessage(u.chatMessage.Name, "Set your trip in order to use this command", u.chatMessage.IsWhisper)
		return
	}

	reason := strings.Join(u.chatMessage.GetArguments()[1:], " ")
	for user := range *u.engine.GetActiveUsers() {
		if user.Trip == u.chatMessage.Trip {
			u.engine.AddAfkUser(user, reason)
		}
	}

	_, _ = u.engine.SendChatMessage(u.chatMessage.Name, " is afk", u.chatMessage.IsWhisper)
}
