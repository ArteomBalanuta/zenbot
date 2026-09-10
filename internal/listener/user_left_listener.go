package listener

import (
	"fmt"
	"log"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type UserLeftListener struct {
	e common.Engine
}

func (l *UserLeftListener) Notify(jsonMessage string) {
	u, err := model.GetUser(jsonMessage)
	if err != nil {
		fmt.Println("Coudn't Remove active user, Error:", err)
		return
	}

	usr := l.e.GetActiveUserByName(u.Name)
	if usr == nil {
		log.Printf("Ignoring leave for unknown user: %s", u.Name)
		return
	}
	if _, err := l.e.LogPresence(usr.Trip, usr.Name, usr.Hash, "left", l.e.GetChannel()); err != nil {
		log.Printf("could not audit leaving user: %v", err)
	}

	l.e.RemoveActiveUser(usr)
	log.Printf("User left: %s", u.Name)
}

func NewUserLeftListener(e common.Engine) *UserLeftListener {
	return &UserLeftListener{
		e: e,
	}
}
