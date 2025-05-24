package core

import (
	"log"
	"strings"
	"zenbot/bot/model"
)

type ChatMessageListener struct {
	engine *Engine
}

func NewChatMessageListener(e *Engine) *ChatMessageListener {
	return &ChatMessageListener{
		engine: e,
	}
}

func (u *ChatMessageListener) Notify(jsonText string) {
	log.Println("chat json message: ", jsonText)
	var chatMessage = model.FromJson(jsonText)

	/* bot owns this message */
	// if u.engine.Name == chatMessage.Name {
	// 	return
	// }

	var user *model.User
	for x, _ := range u.engine.ActiveUsers {
		if strings.EqualFold(u.engine.Name, chatMessage.Name) {
			user = &x
			break
		}
	}

	//TODO: log somewhere
	log.Println("log author of chatmsg: ", user)

	//TODO: deliver mail for user if present
	//TODO: if afk notify; if not afk notify

	if !strings.HasPrefix(chatMessage.Text, u.engine.prefix) {
		return
	}

	//TODO: check if authorized to run the cmd

	var cmdText string = ExtractCommandText(chatMessage.Text, u.engine.prefix)
	log.Println("Extracted cmd: ", cmdText)

	var cmd Command = BuildCommand(cmdText, u.engine, chatMessage)
	if cmd == nil {
		return
	}
	cmd.Execute()
}
