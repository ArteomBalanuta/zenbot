package command

import (
	"strings"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type Say struct {
	AccessLevel model.Role
	engine      common.Engine
	chatMessage *model.ChatMessage
}

func (u *Say) GetAliases() []string {
	return []string{"say", "echo"}
}

func (u *Say) GetRole() *model.Role {
	return &u.AccessLevel
}

func (u *Say) NewInstance(engine common.Engine, chatMessage *model.ChatMessage) common.Command {
	return &Say{
		AccessLevel: model.USER,
		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *Say) Execute() {
	message := strings.Join(u.chatMessage.GetArguments()[1:], " ") + " "
	if user := u.engine.GetActiveUserByName(u.chatMessage.Name); user == nil || !u.engine.IsUserAuthorized(user, rolePtr(model.ADMIN)) {
		message = strings.Map(func(r rune) rune {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
				return r
			}
			return -1
		}, message)
	}

	// Saturn deliberately sends say output as a public, bot-authored message.
	_, _ = u.engine.SendChatMessage("", message, false)
}
