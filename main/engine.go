package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
)

type Engine struct {
	outMessageQueue chan string
	inMessageQueue  chan string
	activeUsers     map[User]struct{}
	hcConnection    *Connection
	msgListener     *ServerMessageListener
}

func NewEngine() *Engine {
	addr := flag.String("addr", "hack.chat", "http service address")
	u := url.URL{Scheme: "wss", Host: *addr, Path: "/chat-ws"}

	e := &Engine{
		outMessageQueue: make(chan string, 256),
		inMessageQueue:  make(chan string, 256),
		activeUsers:     make(map[User]struct{}),
	}

	e.msgListener = NewUserMessageListener(e)
	e.hcConnection = NewConnection(u.String(), e.msgListener)

	return e
}

func (e *Engine) Start() {
	c := e.hcConnection

	c.wg.Add(1)
	go c.Connect()

	for {
		if c.joinedRoom == false && c.IsConnected() {
			channel := "programming"
			nick := "goblood"
			password := "42"
			joinPayload := fmt.Sprintf(`{ "cmd": "join", "channel": "%s", "nick": "%s#%s" }`, channel, nick, password)

			c.Write(joinPayload)

			log.Println("Joining the room: ", channel)
			c.joinedRoom = true

			break
		}
	}
}

func (e *Engine) Stop() {
	e.hcConnection.Close()
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
		var users *[]User = GetUsers(jsonMessage)
		for _, user := range *users {
			e.activeUsers[user] = struct{}{}
		}

		for user, _ := range e.activeUsers {
			log.Println("Active user: ", user.Name)
		}
	case "onlineAdd":
		// userJoinedListener.notify(jsonText)
	case "onlineRemove":
		// userLeftListener.notify(jsonText)
	case "chat":
		// chatMessageListener.notify(jsonText)
	case "info":
		// infoMessageListener.notify(jsonText)
	case "session":
	default:
		log.Println("Non functional payload: ", jsonMessage)
	}

}
