package core

import (
	"log"
	"zenbot/internal/model"
)

type Command interface {
	Execute()
	GetRole() *model.Role
	GetAliases() []string
	NewInstance(e *Engine, m *model.ChatMessage) Command
}

type CommandMetadata struct {
	Alias   string
	Command func(msg *model.ChatMessage) Command
}

func BuildCommand(alias string, e *Engine, msg *model.ChatMessage) Command {
	command, exists := e.EnabledCommands[alias]
	if !exists {
		log.Println("Unknown command")
	} else {
		log.Println("Returning command: ", alias)
		return command.Command(msg)
	}

	return nil
}
