package core

import (
	"zenbot/internal/model"
)

type Command interface {
	Execute()
	GetRole() *model.Role
	GetAliases() []string
	NewInstance(e *Engine, m *model.ChatMessage) Command
}
