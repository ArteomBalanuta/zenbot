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
	engine := u.engine

	log.Println("chat json message: ", jsonText)
	var chatMessage = model.FromJson(jsonText)

	/* bot owns this message */
	// if u.engine.Name == chatMessage.Name {
	// 	return
	// }

	var author *model.User
	for x, _ := range engine.ActiveUsers {
		if strings.EqualFold(x.Name, chatMessage.Name) {
			author = &x
			break
		}
	}

	//TODO: deliver mail for user if present
	//TODO: if afk notify; if not afk notify

	isCommand := strings.HasPrefix(chatMessage.Text, engine.prefix)
	if !isCommand {
		return
	}

	var cmdText string = ParseCommandText(chatMessage.Text, engine.prefix)
	var cmd Command = BuildCommand(cmdText, engine, chatMessage)

	if !engine.SecurityService.IsAuthorized(author, cmd.GetRole()) {
		log.Printf("User is [NOT] Authorized to run command: [%s], hash: %s, trip: %s, name: %s", cmdText, author.Hash, author.Trip, author.Name)
		return
	}

	log.Printf("User [IS] whitelisted, hash: %s, trip: %s, name: %s", author.Hash, author.Trip, author.Name)

	if cmd == nil {
		return
	}
	cmd.Execute()
}
