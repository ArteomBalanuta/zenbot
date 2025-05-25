package command

import (
	"log"
	"strings"
	"zenbot/bot/model"
)

type SayTwice struct {
	Aliases     [10]string `aliases:"saytwice"`
	AccessLevel model.Role

	engine      model.EngineInterface
	chatMessage *model.ChatMessage
}

func NewSayTwice(engine model.EngineInterface, chatMessage *model.ChatMessage) *SayTwice {
	return &SayTwice{
		Aliases:     [10]string{"saytwice", "s"}, //TODO: add check against this
		AccessLevel: model.USER,                  //TODO: add check against this

		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *SayTwice) Execute() {
	log.Println("In say executing")
	var argArr = u.chatMessage.GetArguments()[1:]
	str := strings.Join(argArr, " ")

	u.engine.EnqueueMessageForSending(str)
	u.engine.EnqueueMessageForSending(str)
}
