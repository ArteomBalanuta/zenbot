package contracts

import "zenbot/bot/model"

type Command interface {
	Execute()
	GetRole() *model.Role
	GetAliases() []string
	NewInstance(e EngineInterface, m *model.ChatMessage) Command
}
