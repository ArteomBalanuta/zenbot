package listener

import (
	"fmt"
	"log"
	"strings"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type UserChatListener struct {
	engine common.Engine
}

func NewUserChatListener(e common.Engine) *UserChatListener {
	return &UserChatListener{
		engine: e,
	}
}

func (u *UserChatListener) Notify(jsonText string) {
	engine := u.engine

	var chatMessage = model.FromJson[model.ChatMessage](jsonText)

	_, err := engine.LogMessage(chatMessage.Text, chatMessage.Name, chatMessage.Hash, chatMessage.Text, engine.GetChannel())
	if err != nil {
		fmt.Println("ERROR logging message:", err)
	}

	/* bot owned message. cmd self invocation is fun. for now ignore it */
	if u.engine.GetName() == chatMessage.Name {
		return
	}

	var author *model.User
	for au, _ := range *engine.GetActiveUsers() {
		if strings.EqualFold(au.Name, chatMessage.Name) {
			author = au
			break
		}
	}

	//TODO: deliver mail for user if present
	//

	//TODO: if afk notify; if not afk notify
	u.engine.RemoveIfAfk(author)
	u.engine.NotifyAfkIfMentioned(chatMessage)

	isCommand := strings.HasPrefix(chatMessage.Text, engine.GetPrefix())
	if !isCommand {
		return
	}

	var cmdText = common.ParseCommandText(chatMessage.Text, engine.GetPrefix())
	var cmd = common.BuildCommand(cmdText, engine, chatMessage)
	if cmd == nil {
		log.Printf("Command: %s, not found. ", cmdText)
		return
	}

	if !engine.IsUserAuthorized(author, cmd.GetRole()) {
		log.Printf("User is [NOT] Authorized to run command: [%s], hash: %s, trip: %s, name: %s", cmdText, author.Hash, author.Trip, author.Name)
		engine.SendMessage(author.Name, fmt.Sprintf(" you are not authorized to run: %s command.", cmdText), chatMessage.IsWhisper)
		return
	}

	log.Printf("User [IS] whitelisted, hash: %s, trip: %s, name: %s", author.Hash, author.Trip, author.Name)
	cmd.Execute()
}
