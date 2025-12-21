package command

import (
	"fmt"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type Ban struct {
	AccessLevel model.Role
	engine      common.Engine
	chatMessage *model.ChatMessage
}

func (u *Ban) GetAliases() []string {
	return []string{"ban", "b"}
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
	target := u.chatMessage.GetArguments()[1:][0]

	user := u.engine.GetActiveUserByName(target)
	if user != nil {
		u.engine.Ban(target)
		u.engine.SendChatMessage(u.chatMessage.Name, fmt.Sprintf(" user with hash: %s banned", user.Hash), u.chatMessage.IsWhisper)
	} else {
		u.engine.SendChatMessage(u.chatMessage.Name, " user not found", u.chatMessage.IsWhisper)
	}
}
