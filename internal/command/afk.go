package core

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
	return []string{"afk", "away"}
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
	var reason = strings.Join(u.chatMessage.GetArguments()[1:], " ")

	for user := range *u.engine.GetActiveUsers() {
		if user.Name == u.chatMessage.Name {
			u.engine.AddAfkUser(user, reason)
		}
	}

	u.engine.SendMessage(u.chatMessage.Name, " is afk.", u.chatMessage.IsWhisper)
}
