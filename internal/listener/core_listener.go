package listener

import (
	"log"
	"zenbot/internal/common"
)

type CoreListener struct {
	engine common.Engine
}

func NewCoreListener(e *common.Engine) *CoreListener {
	return &CoreListener{
		engine: *e,
	}
}

func (u *CoreListener) Notify(s string) {
	log.Println("Incoming message: ", s)
	u.engine.DispatchMessage(s)
}
