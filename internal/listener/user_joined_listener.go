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

type JoinAutomation interface {
	OnJoin(context.Context, *model.User)
}

type UserJoinedListener struct {
	e          common.Engine
	automation JoinAutomation
	autoMove   JoinAutomation
}

func (l *UserJoinedListener) Notify(jsonMessage string) {
	u, err := model.GetUser(jsonMessage)
	if err != nil {
		fmt.Println("Coudn't Add active user, Error:", err)
		return
	}
	l.e.AddActiveUser(u)
	if l.automation != nil {
		l.automation.OnJoin(context.Background(), u)
	}
	l.shareUserInfo(u)
	l.kickIfShadowBanned(u)
	if _, err := l.e.LogPresence(u.Trip, u.Name, u.Hash, "joined", l.e.GetChannel()); err != nil {
		log.Printf("could not audit joined user: %v", err)
	}
	l.notifySeenRecently(u)
	if l.autoMove != nil {
		l.autoMove.OnJoin(context.Background(), u)
	}
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
		if !l.e.IsSubscribedTrip(active.Trip) {
			continue
		}
		_, _ = l.e.SendAddressedMessage(active.Name, " -\\n\\n"+data, true)
	}
}

func (l *UserJoinedListener) kickIfShadowBanned(joined *model.User) {
	provider, ok := l.e.(interface{ ServiceBundle() *service.Bundle })
	if !ok || provider.ServiceBundle() == nil || provider.ServiceBundle().ShadowBans == nil {
		return
	}
	banned, err := provider.ServiceBundle().ShadowBans.Matches(context.Background(), joined)
	if err != nil {
		log.Printf("could not check joined user shadow-ban: %v", err)
		return
	}
	if !banned {
		return
	}
	moderation, ok := l.e.(common.ModerationOperations)
	if !ok {
		log.Printf("cannot kick shadow-banned user %q: moderation capability unavailable", joined.Name)
		return
	}
	if err := moderation.KickNick(context.Background(), common.NickTarget(joined.Name)); err != nil {
		log.Printf("could not kick shadow-banned user %q: %v", joined.Name, err)
	}
}

func (l *UserJoinedListener) notifySeenRecently(joined *model.User) {
	if joined == nil || joined.Isme || strings.EqualFold(joined.Name, l.e.GetName()) {
		return
	}
	if matcher, ok := l.e.(common.BotIdentityMatcher); ok && matcher.IsManagedBotName(joined.Name) {
		return
	}
	provider, ok := l.e.(interface{ ServiceBundle() *service.Bundle })
	if !ok || provider.ServiceBundle() == nil || provider.ServiceBundle().Users == nil {
		return
	}
	message, err := provider.ServiceBundle().Users.SeenRecently(context.Background(), joined)
	if err != nil {
		log.Printf("could not query recent aliases: %v", err)
		return
	}
	if message != "" {
		_, _ = l.e.SendChatMessage("", message, false)
	}
}

func NewUserJoinedListener(e common.Engine) *UserJoinedListener {
	return NewUserJoinedListenerWithAutomation(e, nil)
}
func NewUserJoinedListenerWithAutomation(e common.Engine, automation JoinAutomation) *UserJoinedListener {
	return NewUserJoinedListenerWithAutomations(e, automation, nil)
}

func NewUserJoinedListenerWithAutomations(e common.Engine, automation, autoMove JoinAutomation) *UserJoinedListener {
	return &UserJoinedListener{e: e, automation: automation, autoMove: autoMove}
}
