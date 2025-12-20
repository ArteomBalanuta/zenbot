package core

import (
	"fmt"
	"log"
	"strings"
	"zenbot/internal/model"
)

type UserChatListener struct {
	engine *Engine
}

func NewUserChatListener(e *Engine) *UserChatListener {
	return &UserChatListener{
		engine: e,
	}
}

func (u *UserChatListener) Notify(jsonText string) {
	engine := u.engine

	var chatMessage = model.FromJson[model.ChatMessage](jsonText)

	_, err := engine.Repository.LogMessage(chatMessage.Text, chatMessage.Name, chatMessage.Hash, chatMessage.Text, engine.Channel)
	if err != nil {
		fmt.Println("ERROR logging message:", err)
		return
	}

	/* bot owned message. cmd self invocation is fun. for now ignore it */
	if u.engine.Name == chatMessage.Name {
		return
	}

	var author *model.User
	for au, _ := range engine.ActiveUsers {
		if strings.EqualFold(au.Name, chatMessage.Name) {
			author = au
			break
		}
	}

	//TODO: deliver mail for user if present
	//TODO: if afk notify; if not afk notify

	isCommand := strings.HasPrefix(chatMessage.Text, engine.prefix)
	if !isCommand {
		return
	}

	var cmdText = ParseCommandText(chatMessage.Text, engine.prefix)
	var cmd = BuildCommand(cmdText, engine, chatMessage)
	if cmd == nil {
		log.Printf("Command: %s, not found. ", cmdText)
		return
	}

	if !engine.SecurityService.IsAuthorized(author, cmd.GetRole()) {
		log.Printf("User is [NOT] Authorized to run command: [%s], hash: %s, trip: %s, name: %s", cmdText, author.Hash, author.Trip, author.Name)
		return
	}

	log.Printf("User [IS] whitelisted, hash: %s, trip: %s, name: %s", author.Hash, author.Trip, author.Name)
	cmd.Execute()
}
