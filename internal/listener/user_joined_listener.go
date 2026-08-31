package listener

import (
	"context"
	"fmt"
	"log"
	"strings"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

type UserJoinedListener struct{ e common.Engine }

func (l *UserJoinedListener) Notify(jsonMessage string) {
	u, err := model.GetUser(jsonMessage)
	if err != nil {
		fmt.Println("Coudn't Add active user, Error:", err)
		return
	}
	l.e.AddActiveUser(u)
	l.shareUserInfo(u)
	l.e.LogPresence(u.Trip, u.Name, u.Hash, "joined", l.e.GetChannel())
	log.Printf("User joined: %s", u.Name)
}

func (l *UserJoinedListener) shareUserInfo(joined *model.User) {
	provider, ok := l.e.(interface{ ServiceBundle() *service.Bundle })
	if !ok || provider.ServiceBundle() == nil || provider.ServiceBundle().Users == nil || provider.ServiceBundle().Users.Queries == nil {
		return
	}
	if len(l.e.GetSubscribedTrips()) == 0 {
		return
	}
	data, err := provider.ServiceBundle().Users.BasicUserData(context.Background(), joined.Hash, joined.Trip)
	if err != nil {
		log.Printf("could not query joined user data: %v", err)
		return
	}
	for active := range *l.e.GetActiveUsers() {
		// Subscription matching is intentionally case-insensitive, as in Saturn.
		if !l.e.IsSubscribedTrip(active.Trip) || !strings.EqualFold(active.Trip, joined.Trip) {
			continue
		}
		_, _ = l.e.SendAddressedMessage(active.Name, " -\\n\\n"+data, true)
	}
}

func NewUserJoinedListener(e common.Engine) *UserJoinedListener { return &UserJoinedListener{e: e} }
