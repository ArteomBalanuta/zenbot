package core

import (
	"log"
)

type CoreListener struct {
	engine *Engine
}

func NewCoreListener(e *Engine) *CoreListener {
	return &CoreListener{
		engine: e,
	}
}

func (u *CoreListener) Notify(s string) {
	log.Println("Incoming message: ", s)
	u.engine.DispatchMessage(s)
}
