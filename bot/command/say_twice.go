package command

import (
	"strings"
	"zenbot/bot/contracts"
	"zenbot/bot/model"
)

type SayTwice struct {
	AccessLevel model.Role
	engine      contracts.EngineInterface
	chatMessage *model.ChatMessage
}

func (u *SayTwice) GetAliases() []string {
	return []string{"sudosay"}
}

func (u *SayTwice) NewInstance(engine contracts.EngineInterface, chatMessage *model.ChatMessage) contracts.Command {
	println("New SayTwice instance")
	return &SayTwice{
		AccessLevel: model.ADMIN,
		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *SayTwice) GetRole() *model.Role {
	return &u.AccessLevel
}

func (u *SayTwice) Execute() {
	var argArr = u.chatMessage.GetArguments()[1:]
	str := strings.Join(argArr, " ")

	u.engine.EnqueueMessageForSending(str)
	u.engine.EnqueueMessageForSending(str)
	u.engine.EnqueueMessageForSending(str)
}
