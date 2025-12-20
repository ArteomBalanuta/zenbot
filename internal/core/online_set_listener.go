package core

import (
	"zenbot/internal/model"
)

type OnlineSetListener struct {
	e        *Engine
	callback func(*Engine)
}

func (l *OnlineSetListener) Notify(jsonMessage string) {
	var users = model.GetUsers(jsonMessage)
	for _, user := range users {
		l.e.ActiveUsers[user] = struct{}{}
	}

	// our callback for replica,zombie instances
	if l.callback != nil {
		l.callback(l.e)
	}
}

func NewOnlineSetListener(e *Engine, callback func(*Engine)) *OnlineSetListener {
	return &OnlineSetListener{
		e:        e,
		callback: callback,
	}
}
