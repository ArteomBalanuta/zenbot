package main

import (
	"log"
)

type UserMessageListener struct {
	engine *Engine
}

func NewUserMessageListener(e *Engine) *UserMessageListener {
	return &UserMessageListener{
		engine: e,
	}
}

func (u *UserMessageListener) notify(s string) {
	log.Println("Incoming message: ", s)
}
