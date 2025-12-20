package listener

import (
	"fmt"
	"log"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type UserJoinedListener struct {
	e common.Engine
}

func (l *UserJoinedListener) Notify(jsonMessage string) {
	u, err := model.GetUser(jsonMessage)
	if err != nil {
		fmt.Println("Coudn't Add active user, Error:", err)
		return
	}
	l.e.AddActiveUser(u)
	l.e.LogPresence(u.Trip, u.Name, u.Hash, "joined", l.e.GetChannel())
	log.Printf("User joined: %s", u.Name)
}

func NewUserJoinedListener(e *common.Engine) *UserJoinedListener {
	return &UserJoinedListener{
		e: *e,
	}
}
