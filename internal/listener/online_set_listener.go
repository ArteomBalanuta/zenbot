package listener

import (
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type OnlineSetListener struct {
	e        common.Engine
	callback func(*common.Engine)
}

func (l *OnlineSetListener) Notify(jsonMessage string) {
	var users = model.GetUsers(jsonMessage)
	for _, user := range users {
		(*l.e.GetActiveUsers())[user] = struct{}{}
	}

	// our callback for replica,zombie instances
	if l.callback != nil {
		l.callback(&l.e)
	}
}

func NewOnlineSetListener(e *common.Engine, callback func(*common.Engine)) *OnlineSetListener {
	return &OnlineSetListener{
		e:        *e,
		callback: callback,
	}
}
