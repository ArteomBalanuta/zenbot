package core

import (
	"fmt"
	"log"
	"strings"
	"zenbot/bot/command"
	"zenbot/bot/contracts"
	"zenbot/bot/model"
)

func ParseCommandText(text, prefix string) string {
	afterPrefix := text[len(prefix):]
	fields := strings.Fields(afterPrefix)
	log.Println("Extracted cmd: ", fields[0])

	return fields[0]
}

type CommandMetadata struct {
	Alias   string
	Command contracts.Command
}

var EnabledCommands = map[string]CommandMetadata{}

func RegisterCommand[T contracts.Command](e *Engine, msg *model.ChatMessage) {
	var command T // equivalent to T{}
	aliases := command.GetAliases()
	var cmd = command.NewInstance(e, msg)
	for _, alias := range aliases {
		EnabledCommands[alias] = CommandMetadata{
			Alias:   alias,
			Command: cmd,
		}
	}

	fmt.Printf("Registered command with aliases: %v\n", aliases)
}

func BuildCommand(alias string, e *Engine, msg *model.ChatMessage) contracts.Command {
	RegisterCommand[*command.SayTwice](e, msg)
	RegisterCommand[*command.Say](e, msg)

	command, exists := EnabledCommands[alias]
	if !exists {
		log.Println("Unknown command")
	} else {
		log.Println("Returning command: ", alias)
		return command.Command
	}

	return nil
}
