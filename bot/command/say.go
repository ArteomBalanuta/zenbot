package command

import (
	"strings"
	"zenbot/bot/model"
)

type Say struct {
	Aliases     [10]string
	AccessLevel model.Role

	engine      model.EngineInterface
	chatMessage *model.ChatMessage
}

func NewSay(engine model.EngineInterface, chatMessage *model.ChatMessage) *Say {
	return &Say{
		Aliases:     [10]string{"say", "s"},
		AccessLevel: model.USER, //TODO: add check agaonst this

		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *Say) Execute() {
	var argArr = u.chatMessage.GetArguments()[1:]
	str := strings.Join(argArr, " ")

	u.engine.EnqueueMessageForSending(str)
}

//[10]string {"1","2","3"}
