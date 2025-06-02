package core

import (
	"fmt"
	"log"
	"zenbot/bot/model"
)

type UserLeftListener struct {
	e *Engine
}

func (l *UserLeftListener) Notify(jsonMessage string) {
	u, err := model.GetUser(jsonMessage)
	if err != nil {
		fmt.Println("Coudn't Remove active user, Error:", err)
		return
	}

	usr := l.e.GetUserByName(u.Name)
	l.e.Repository.LogPresence(usr.Trip, usr.Name, usr.Hash, "left", l.e.Channel)

	l.e.RemoveActiveUser(usr)
	log.Printf("User left: %s", u.Name)
}

func NewUserLeftListener(e *Engine) *UserLeftListener {
	return &UserLeftListener{
		e: e,
	}
}
