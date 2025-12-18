package core

import "zenbot/bot/contracts"
import "zenbot/bot/model"

type Command interface {
	Execute()
	GetRole() *model.Role
	GetAliases() []string
	NewInstance(e contracts.EngineInterface, m *model.ChatMessage) Command
}
