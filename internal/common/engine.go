package common

import "zenbot/internal/model"

type Engine interface {
	Start()
	Stop()

	DispatchMessage(jsonMessage string)
	SendRawMessage(message string)
	SendMessage(author, message string, IsWhisper bool) (string, error)

	AddActiveUser(joined *model.User)
	RemoveActiveUser(left *model.User)

	AddAfkUser(u *model.User, reason string)
	GetAfkUsers() *map[*model.User]string

	GetActiveUserByName(name string) *model.User
	GetActiveUsers() *map[*model.User]struct{}

	GetPrefix() string
	GetName() string
	GetChannel() string

	SetName(n string)

	RegisterCommand(c *Command)
	GetEnabledCommands() *map[string]CommandMetadata

	SetOnlineSetListener(l *Listener)

	LogMessage(trip, name, hash, message, channel string) (int64, error)
	LogPresence(trip, name, hash, eventType, channel string) (int64, error)

	SetLastKickedUser(name string)
	SetLastKickedChannel(channel string)

	IsUserAuthorized(u *model.User, r *model.Role) bool

	NotifyAfkIfMentioned(m *model.ChatMessage)
	RemoveIfAfk(u *model.User)
}
