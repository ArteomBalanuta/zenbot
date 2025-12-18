package core

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	// "zenbot/bot/command"
	"zenbot/bot/model"
	"zenbot/bot/repository"
	"zenbot/bot/service"
)

type Engine struct {
	eType    model.EngineType
	prefix   string
	Channel  string
	Name     string
	Password string

	OutMessageQueue chan string
	ActiveUsers     map[*model.User]struct{}
	HcConnection    *Connection
	Repository      *repository.Repository

	CoreListener       *CoreListener
	OnlineSetListener  *OnlineSetListener
	UserJoinedListener *UserJoinedListener
	UserChatListener   *UserChatListener
	UserLeftListener   *UserLeftListener

	SecurityService *service.SecurityService

	EnabledCommands map[string]CommandMetadata
}

func (e *Engine) Start() {
	c := e.HcConnection

	c.Wg.Add(1)
	go c.Connect()

	for {
		if c.joinedRoom == false && c.IsWsConnected() {
			joinPayload := fmt.Sprintf(`{ "cmd": "join", "channel": "%s", "nick": "%s#%s" }`, e.Channel, e.Name, e.Password)

			c.Write(joinPayload)

			log.Println("Joining the room: ", e.Channel)
			c.joinedRoom = true

			break
		}
	}

	e.RegisterCommand(&List{})
	e.RegisterCommand(&Say{})

	go e.StartSharingMessages()
}

func (e *Engine) Stop() {
	e.HcConnection.Close()
}

func (e *Engine) DispatchMessage(jsonMessage string) {
	// Parse into a map
	var data map[string]interface{}
	err := json.Unmarshal([]byte(jsonMessage), &data)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return
	}

	// Extract "cmd"
	cmd, ok := data["cmd"].(string)
	if !ok {
		fmt.Println("Key 'cmd' not found or not a string")
		return
	}

	fmt.Println("Command:", cmd)

	switch cmd {
	case "join":
	case "onlineSet":
		e.OnlineSetListener.Notify(jsonMessage)
	case "onlineAdd":
		e.UserJoinedListener.Notify(jsonMessage)
	case "onlineRemove":
		e.UserLeftListener.Notify(jsonMessage)
	case "chat":
		e.UserChatListener.Notify(jsonMessage)
	case "info":
		log.Printf("info: %s", jsonMessage)
	case "session":
	default:
		log.Printf("Non functional payload: %s", jsonMessage)
	}
}

func (e *Engine) SendRawMessage(message string) {
	e.OutMessageQueue <- message
}

func (e *Engine) SendMessage(author, message string, IsWhisper bool) error {
	if strings.TrimSpace(author) == "" {
		return fmt.Errorf("author can't be null")
	}

	if IsWhisper {
		message = "/whisper @" + author + " " + message
	} else {
		message = "@" + author + " " + message
	}

	e.OutMessageQueue <- message
	return nil
}

func (e *Engine) StartSharingMessages() {
	go func() {
		for msg := range e.OutMessageQueue {
			chatPayload := fmt.Sprintf(`{ "cmd": "chat", "text": "%s"}`, escapeJSON(msg))

			log.Println("sending: ", chatPayload)
			e.HcConnection.Write(chatPayload)
		}
	}()
}

func escapeJSON(input string) string {
	escaped, _ := json.Marshal(input)
	// Remove the surrounding quotes
	s := string(escaped[1 : len(escaped)-1])

	// Restore specific whitespace characters
	s = strings.ReplaceAll(s, `\n`, "\\n")
	s = strings.ReplaceAll(s, `\t`, "\\t")
	s = strings.ReplaceAll(s, `\r`, "\\r")

	return s
}

func (e *Engine) AddActiveUser(joined *model.User) {
	e.ActiveUsers[joined] = struct{}{}
}

func (e *Engine) RemoveActiveUser(left *model.User) {
	for u := range e.ActiveUsers {
		if u.Name == left.Name {
			delete(e.ActiveUsers, u)
			break
		}
	}
}

func (e *Engine) GetUserByName(name string) *model.User {
	for u := range e.ActiveUsers {
		if u.Name == name {
			return u
		}
	}

	return nil
}

func (e *Engine) GetActiveUsers() map[*model.User]struct{} {
	return e.ActiveUsers
}

func (e *Engine) GetChannel() string {
	return e.Channel
}

func ParseCommandText(text, prefix string) string {
	afterPrefix := text[len(prefix):]
	fields := strings.Fields(afterPrefix)
	log.Println("Extracted cmd: ", fields[0])

	return fields[0]
}

type CommandMetadata struct {
	Alias   string
	Command func(msg *model.ChatMessage) Command
}

func (e *Engine) RegisterCommand(c Command) {
	aliases := c.GetAliases()
	var constructorFn = func(msg *model.ChatMessage) Command {
		return c.NewInstance(e, msg)
	}

	for _, alias := range aliases {
		e.EnabledCommands[alias] = CommandMetadata{
			Alias:   alias,
			Command: constructorFn,
		}
	}

	fmt.Printf("Registered command with aliases: %v\n", aliases)
}
