package common

import (
	"log"
	"sort"
	"strings"
	"zenbot/internal/model"
)

type Command interface {
	Execute()
	GetRole() *model.Role
	GetAliases() []string
	NewInstance(e Engine, m *model.ChatMessage) Command
}

// CommandAuthorizer supplies a command-specific authorization predicate.
type CommandAuthorizer interface {
	Authorize(*model.User) bool
}

// ContextualCommandAuthorizer evaluates policy against the engine view used by
// the active dispatch path. This preserves capabilities added by engine
// decorators instead of silently falling back to an embedded base engine.
type ContextualCommandAuthorizer interface {
	AuthorizeWithEngine(Engine, *model.User) bool
}

// IsCommandAuthorized applies the most specific command policy available,
// then falls back to the engine's role check.
func IsCommandAuthorized(engine Engine, command Command, user *model.User) bool {
	if engine == nil || command == nil || user == nil {
		return false
	}
	if authorizer, ok := command.(ContextualCommandAuthorizer); ok {
		return authorizer.AuthorizeWithEngine(engine, user)
	}
	if authorizer, ok := command.(CommandAuthorizer); ok {
		return authorizer.Authorize(user)
	}
	return engine.IsUserAuthorized(user, command.GetRole())
}

type CommandMetadata struct {
	Alias   string
	Command func(msg *model.ChatMessage) Command
}

func BuildCommand(alias string, e Engine, msg *model.ChatMessage) Command {
	if indexed, ok := e.(interface {
		LookupCommand(string) (CommandMetadata, bool)
	}); ok {
		command, exists := indexed.LookupCommand(alias)
		if exists && command.Command != nil {
			return command.Command(msg)
		}
		return nil
	}
	snapshot := e.GetEnabledCommands()
	if snapshot == nil {
		return nil
	}
	commands := *snapshot
	command, exists := commands[strings.ToLower(strings.TrimSpace(alias))]
	if !exists {
		wanted := commandAnagramKey(alias)
		for candidate, metadata := range commands {
			if commandAnagramKey(candidate) == wanted {
				// Map-only engines cannot prove that two aliases share an owner.
				if exists {
					return nil
				}
				command, exists = metadata, true
			}
		}
	}
	if !exists || command.Command == nil {
		log.Println("Unknown command")
	} else {
		log.Println("Returning command: ", alias)
		return command.Command(msg)
	}

	return nil
}

func commandAnagramKey(value string) string {
	runes := []rune(strings.ToLower(strings.TrimSpace(value)))
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
	return string(runes)
}
