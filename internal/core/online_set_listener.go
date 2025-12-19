package core

import (
	// "log"
	"fmt"
	"zenbot/internal/model"
)

type OnlineSetListener struct {
	e        *Engine
	callback func(*Engine)
}

func (l *OnlineSetListener) Notify(jsonMessage string) {
	var users []*model.User = model.GetUsers(jsonMessage)
	for _, user := range users {
		l.e.ActiveUsers[user] = struct{}{}
	}

	// our callback for ZOMBIE instances
	if l.callback != nil {
		fmt.Println("OnlineSetListener callback is present, executing...")
		l.callback(l.e)
	}

	// for user, _ := range l.e.ActiveUsers {
	// 	log.Println("Active user: ", user.Name)
	// }
}

func NewOnlineSetListener(e *Engine, callback func(*Engine)) *OnlineSetListener {
	return &OnlineSetListener{
		e:        e,
		callback: callback,
	}
}
