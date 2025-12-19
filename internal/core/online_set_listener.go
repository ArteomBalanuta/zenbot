package core

import (
	// "log"
	"zenbot/internal/model"
)

type OnlineSetListener struct {
	e *Engine
}

func (l *OnlineSetListener) Notify(jsonMessage string) {
	var users []*model.User = model.GetUsers(jsonMessage)
	for _, user := range users {
		l.e.ActiveUsers[user] = struct{}{}
	}

	// for user, _ := range l.e.ActiveUsers {
	// 	log.Println("Active user: ", user.Name)
	// }
}

func NewOnlineSetListener(e *Engine) *OnlineSetListener {
	return &OnlineSetListener{
		e: e,
	}
}
