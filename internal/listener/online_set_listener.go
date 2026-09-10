package listener

import (
	"encoding/json"
	"log"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type activeSnapshotReplacer interface{ ReplaceActiveUsers([]*model.User) }
type OnlineSetListener struct {
	e        common.Engine
	callback func(common.Engine)
}

func (l *OnlineSetListener) Notify(payload string) {
	var root struct {
		Users  []*model.User `json:"users"`
		Legacy []*model.User `json:"Users"`
	}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		log.Printf("malformed onlineSet payload: %v", err)
		return
	}
	users := root.Users
	if users == nil {
		users = root.Legacy
	}
	for _, u := range users {
		if u == nil || u.Name == "" {
			log.Printf("invalid onlineSet user")
			return
		}
	}
	if r, ok := l.e.(activeSnapshotReplacer); ok {
		r.ReplaceActiveUsers(users)
	} else {
		for _, u := range users {
			l.e.AddActiveUser(u)
		}
	}
	if l.callback != nil {
		l.callback(l.e)
	}
}
func NewOnlineSetListener(e common.Engine, cb func(common.Engine)) *OnlineSetListener {
	return &OnlineSetListener{e: e, callback: cb}
}

// SnapshotOnlineSetListener forwards the original payload without touching
// engine state. Temporary snapshot sessions must not become host listeners.
type SnapshotOnlineSetListener struct{ sink func(string) }

func NewSnapshotOnlineSetListener(sink func(string)) *SnapshotOnlineSetListener {
	return &SnapshotOnlineSetListener{sink: sink}
}
func (l *SnapshotOnlineSetListener) Notify(payload string) {
	var root struct {
		Users  []*model.User `json:"users"`
		Legacy []*model.User `json:"Users"`
	}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return
	}
	users := root.Users
	if users == nil {
		users = root.Legacy
	}
	for _, u := range users {
		if u == nil || u.Name == "" {
			return
		}
	}
	if l.sink != nil {
		l.sink(payload)
	}
}
