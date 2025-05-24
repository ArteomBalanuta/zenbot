package core

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
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

func NewEngine() *Engine {
	addr := flag.String("addr", "hack.chat", "http service address")
	u := url.URL{Scheme: "wss", Host: *addr, Path: "/chat-ws"}

	e := &Engine{
		prefix:   "go ",
		Channel:  "programming",
		Name:     "gobot",
		Password: "",

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

func EnqueueMessageForSending(e *Engine, message string) {
	e.OutMessageQueue <- message
}

func (e *Engine) ShareMessages() {
	msg := <-e.OutMessageQueue
	e.HcConnection.Write(msg)
}
