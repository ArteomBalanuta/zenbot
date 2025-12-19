package core

import (
	"fmt"
	"log"
	"zenbot/internal/model"
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
	l.e.Repository.LogPresence(u.Trip, u.Name, u.Hash, "joined", l.e.Channel)
	log.Printf("User joined: %s", u.Name)
}

func NewUserJoinedListener(e *Engine) *UserJoinedListener {
	return &UserJoinedListener{
		e: e,
	}
}
