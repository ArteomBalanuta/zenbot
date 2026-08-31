package command

import (
	"fmt"
	"strings"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type Ban struct {
	AccessLevel model.Role
	engine      common.Engine
	chatMessage *model.ChatMessage
}

func (u *Ban) GetAliases() []string {
	return []string{"ban"}
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
	arguments := u.chatMessage.GetArguments()
	if len(arguments) < 2 || strings.TrimSpace(arguments[1]) == "" {
		u.engine.SendChatMessage(u.chatMessage.Name, "Example: "+u.engine.GetPrefix()+"ban merc", u.chatMessage.IsWhisper)
		return
	}
	target := strings.TrimPrefix(strings.TrimSpace(arguments[1]), "@")
	if target == "" {
		return
	}

	u.engine.Ban(target)
	u.engine.SendChatMessage(u.chatMessage.Name, fmt.Sprintf("%s has been banned", target), u.chatMessage.IsWhisper)
}
