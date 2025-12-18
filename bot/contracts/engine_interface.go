package contracts

import (
	"zenbot/bot/model"
)

type EngineInterface interface {
	Start()
	Stop()

	SendRawMessage(message string)
	SendMessage(author, message string, IsWhisper bool) error

	GetChannel() string
	GetActiveUsers() map[*model.User]struct{}
}
