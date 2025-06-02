package core

import (
	"log"
	"strings"
	"zenbot/bot/contracts"
	"zenbot/bot/model"
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

	log.Println("chat json message: ", jsonText)
	var chatMessage = model.FromJson(jsonText)

	engine.Repository.LogMessage(chatMessage.Text, chatMessage.Name, chatMessage.Hash, chatMessage.Text, engine.Channel)

	/* bot owns this message so we ignore it */
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

	var cmdText string = ParseCommandText(chatMessage.Text, engine.prefix)
	var cmd contracts.Command = BuildCommand(cmdText, engine, chatMessage)
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
