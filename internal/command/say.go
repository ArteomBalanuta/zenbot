package core

import (
	"log"
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
	println("New instance")
	return &Say{
		AccessLevel: model.USER,
		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *Say) Execute() {
	log.Println("In say executing")
	var argArr = u.chatMessage.GetArguments()[1:]
	str := strings.Join(argArr, " ") // TODO: fix - make sure \n \t are preserved!

	u.engine.SendRawMessage(str)
}
