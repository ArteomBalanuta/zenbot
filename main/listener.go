package main

import (
	"log"
)

type ServerMessageListener struct {
	engine *Engine
}

func NewUserMessageListener(e *Engine) *ServerMessageListener {
	return &ServerMessageListener{
		engine: e,
	}
}

func (u *ServerMessageListener) notify(s string) {
	log.Println("Incoming message: ", s)
	u.engine.DispatchMessage(s)
}
