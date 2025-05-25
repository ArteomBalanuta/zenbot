package core

import (
	"log"
	"reflect"
	"strings"
	"zenbot/bot/command"
	"zenbot/bot/model"
)

func ParseCommandText(text, prefix string) string {
	afterPrefix := text[len(prefix):]
	fields := strings.Fields(afterPrefix)
	log.Println("Extracted cmd: ", fields[0])

	return fields[0]
}

type StructMetadata struct {
	Type reflect.Type
	Info string
	fc   func() Command
}

var EnabledCommands = map[string]StructMetadata{}

func RegisterCommand[T any](constructor func() Command) {
	var zero T
	t := reflect.TypeOf(zero)

	if t.Kind() == reflect.Struct && t.NumField() > 0 {
		field := t.Field(0)
		alias := field.Tag.Get("aliases") // TODO: add support for whitespace separated aliases inside tag
		EnabledCommands[alias] = StructMetadata{
			Type: t,
			Info: alias,
			fc:   constructor,
		}
	}
}

func BuildCommand(alias string, e *Engine, msg *model.ChatMessage) Command {
	// TODO: Move into EnabledCommands() somewhere to engine or config initialization!
	RegisterCommand[command.Say](func() Command {
		return command.NewSay(e, msg)
	})
	RegisterCommand[command.SayTwice](func() Command {
		return command.NewSayTwice(e, msg)
	})

	command := EnabledCommands[alias]
	if command.fc == nil {
		log.Println("Unknown command")
	} else {
		log.Println("Executing command: ", alias)
		return command.fc()
	}

	return nil
}
