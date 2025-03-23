package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
)

type Engine struct {
	outMessageQueue chan string
	inMessageQueue  chan string
	hcConnection    *Connection
	msgListener     *UserMessageListener
}

func NewEngine() *Engine {
	addr := flag.String("addr", "hack.chat", "http service address")
	u := url.URL{Scheme: "wss", Host: *addr, Path: "/chat-ws"}

	e := &Engine{
		outMessageQueue: make(chan string, 256),
		inMessageQueue:  make(chan string, 256),
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
