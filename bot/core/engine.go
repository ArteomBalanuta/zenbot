package core

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"zenbot/bot/config"
	"zenbot/bot/model"
)

type Engine struct {
	prefix   string
	Channel  string
	Name     string
	Password string

	OutMessageQueue     chan string
	ActiveUsers         map[model.User]struct{}
	HcConnection        *Connection
	CoreListener        *CoreListener
	ChatMessageListener *ChatMessageListener
}

func NewEngine(c *config.Config) *Engine {

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
		ActiveUsers:     make(map[model.User]struct{}),
	}

	e.CoreListener = NewCoreListener(e)
	e.ChatMessageListener = NewChatMessageListener(e)

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

	fmt.Println("Command: ", cmd)

	switch cmd {
	case "join":
	case "onlineSet":
		var users *[]model.User = model.GetUsers(jsonMessage)
		for _, user := range *users {
			e.ActiveUsers[user] = struct{}{}
		}

		for user, _ := range e.ActiveUsers {
			log.Println("Active user: ", user.Name)
		}
	case "onlineAdd":
		// userJoinedListener.notify(jsonText)
	case "onlineRemove":
		// userLeftListener.notify(jsonText)
	case "chat":
		e.ChatMessageListener.Notify(jsonMessage)
	case "info":
		// infoMessageListener.notify(jsonText)
	case "session":
	default:
		log.Println("Non functional payload: ", jsonMessage)
	}
}

func (e *Engine) EnqueueMessageForSending(message string) {
	e.OutMessageQueue <- message
}

func (e *Engine) ShareMessages() {
	msg := <-e.OutMessageQueue

	chatPayload := fmt.Sprintf(`{ "cmd": "chat", "text": "%s"}`, msg)

	e.HcConnection.Write(chatPayload)
}
