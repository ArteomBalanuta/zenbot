package listener

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type InfoChatListener struct {
	engine common.Engine
}

func NewInfoChatListener(e common.Engine) *InfoChatListener {
	return &InfoChatListener{
		engine: e,
	}
}

func SliceUpTo(s string, n int) string {
	runes := []rune(s)
	return string(runes[0:n])
}
func SliceDownTo(s string, n int) string {
	runes := []rune(s)
	return string(runes[n+1:])
}

func (u *InfoChatListener) infoToChatMessage(message *model.InfoMessage) model.ChatMessage {
	log.Printf("Received info message: %v, from server\"", message)

	authorName := message.From
	if strings.Contains(message.Text, "whispered:") {
		split := strings.Split(message.Text, fmt.Sprintf("%v", authorName)+" whispered: ")
		text := split[1]

		var author *model.User
		for user := range *u.engine.GetActiveUsers() {
			if user.Name == authorName {
				author = user
				break
			}
		}

		chatMessage := model.ChatMessage{IsWhisper: true, Size: strconv.Itoa(len(text)), Cmd: "", Name: author.Name, Trip: author.Trip, Hash: author.Hash, Text: text}

		log.Printf("Received whisper: %v, from: %v, trip: %v, hash: %v ", text, author.Name, author.Trip, author.Hash)

		return chatMessage
	}

	return model.ChatMessage{}
}

func (u *InfoChatListener) processRename(message *model.InfoMessage) bool {
	e := u.engine
	var text = message.Text
	if strings.Contains(text, " is now ") {
		split := strings.Split(text, " is now ")
		before := split[0]
		after := split[1]

		// renaming self
		if e.GetName() == before {
			e.SetName(after)
		}

		for user := range *e.GetAfkUsers() {
			if user.Name == before {
				user.Name = after
				log.Printf("User renamed from: %s to %s, updated AFK user list", before, after)
				return true
			}
		}
	}

	return false
}

func (u *InfoChatListener) Notify(jsonText string) {
	engine := u.engine
	var infoMessage = model.FromJson[model.InfoMessage](jsonText)

	_, err := engine.LogMessage(infoMessage.Text, infoMessage.Name, "", infoMessage.Text, engine.GetChannel())
	if err != nil {
		fmt.Println("ERROR logging message:", err)
	}

	/* bot owned message. cmd self invocation is fun. for now ignore it */
	if u.engine.GetName() == infoMessage.Name {
		return
	}

	/* kick event */
	if strings.Contains(infoMessage.Text, "was banished to") {
		engine.SetLastKickedUser(SliceUpTo(infoMessage.Text, strings.Index(infoMessage.Text, " was banished")))
		engine.SetLastKickedChannel(SliceDownTo(infoMessage.Text, strings.Index(infoMessage.Text, "?")))
		return
	}

	/* just a rename event */
	if u.processRename(infoMessage) {
		return
	}

	//TODO: code below is almost duplicated in user_chat_listener, mby extract into common func
	chatMessage := u.infoToChatMessage(infoMessage)

	var author *model.User
	for au, _ := range *engine.GetActiveUsers() {
		if strings.EqualFold(au.Name, chatMessage.Name) {
			author = au
			break
		}
	}

	isCommand := strings.HasPrefix(chatMessage.Text, engine.GetPrefix())
	if !isCommand {
		return
	}

	var cmdText = common.ParseCommandText(chatMessage.Text, engine.GetPrefix())
	var cmd = common.BuildCommand(cmdText, engine, &chatMessage)
	if cmd == nil {
		log.Printf("Command: %s, not found. ", cmdText)
		return
	}

	log.Printf("Received whisper cmd: %s", cmdText)

	if !engine.IsUserAuthorized(author, cmd.GetRole()) {
		log.Printf("User is [NOT] Authorized to run command: [%s], hash: %s, trip: %s, name: %s", cmdText, author.Hash, author.Trip, author.Name)
		engine.SendMessage(author.Name, fmt.Sprintf(" you are not authorized to run: %s command.", cmdText), chatMessage.IsWhisper)
		return
	}

	log.Printf("User [IS] whitelisted, hash: %s, trip: %s, name: %s", author.Hash, author.Trip, author.Name)
	cmd.Execute()
}
