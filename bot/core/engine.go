package core

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"zenbot/bot/command"
	"zenbot/bot/config"
	"zenbot/bot/model"
	"zenbot/bot/repository"
	"zenbot/bot/service"
)

type Engine struct {
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
}

func NewEngine(c *config.Config, repository *repository.Repository) *Engine {

	u, err := url.Parse(c.WebsocketUrl)
	if err != nil {
		log.Fatalln("Can't parse websocket URL:", c.WebsocketUrl)
		panic("Error parsing Websocket URL")
	}

	e := &Engine{
		prefix:   c.CmdPrefix,
		Channel:  c.Channel,
		Name:     c.Name,
		Password: c.Password,

		OutMessageQueue: make(chan string, 256),
		ActiveUsers:     make(map[*model.User]struct{}),
	}

	e.Repository = repository
	e.SecurityService = service.NewSecurityService(c)

	e.CoreListener = NewCoreListener(e)
	e.UserChatListener = NewUserChatListener(e)
	e.OnlineSetListener = NewOnlineSetListener(e)
	e.UserJoinedListener = NewUserJoinedListener(e)
	e.UserLeftListener = NewUserLeftListener(e)

	e.HcConnection = NewConnection(u.String(), e.CoreListener)

	return e
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

	RegisterCommand[*command.SayTwice](e)
	RegisterCommand[*command.Say](e)

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

func (e *Engine) EnqueueMessageForSending(message string) {
	e.OutMessageQueue <- message
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
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\r`, "\r")

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
