package core

import (
	"fmt"
	"log"
	"zenbot/bot/model"
)

type UserJoinedListener struct {
	e *Engine
}

func (l *UserJoinedListener) Notify(jsonMessage string) {
	u, err := model.GetUser(jsonMessage)
	if err != nil {
		fmt.Println("Coudn't Add active user, Error:", err)
		return
	}
	l.e.AddActiveUser(u)
	log.Printf("User joined: %s", u.Name)
}

func NewUserJoinedListener(e *Engine) *UserJoinedListener {
	return &UserJoinedListener{
		e: e,
	}
}
