package command

import (
	"strings"
	"zenbot/bot/core"
	"zenbot/bot/model"
)

type Say struct {
	Aliases     [10]string
	AccessLevel model.Role

	engine      core.Engine
	chatMessage model.ChatMessage
}

func NewSay(engine core.Engine, chatMessage model.ChatMessage) *Say {
	return &Say{
		Aliases:     [10]string{"say", "s"},
		AccessLevel: model.USER,

		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *Say) execute() {
	var argArr = u.chatMessage.GetArguments()[1:]
	str := strings.Join(argArr, " ")

	core.EnqueueMessageForSending(&u.engine, str)
}

//[10]string {"1","2","3"}
