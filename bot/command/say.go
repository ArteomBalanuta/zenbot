package command

import (
	"log"
	"strings"
	"zenbot/bot/model"
)

type Say struct {
	Aliases     [10]string `aliases:"say"`
	AccessLevel model.Role

	engine      model.EngineInterface
	chatMessage *model.ChatMessage
}

func NewSay(engine model.EngineInterface, chatMessage *model.ChatMessage) *Say {
	return &Say{
		Aliases:     [10]string{"say", "s"}, //TODO: add check against this
		AccessLevel: model.USER,             //TODO: add check against this

		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *Say) Execute() {
	log.Println("In say executing")
	var argArr = u.chatMessage.GetArguments()[1:]
	str := strings.Join(argArr, " ")

	u.engine.EnqueueMessageForSending(str)
}
