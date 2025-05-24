package core

import (
	"log"
	"strings"
	"zenbot/bot/command"
	"zenbot/bot/model"
)

func ExtractCommandText(text, prefix string) string {
	if !strings.HasPrefix(text, prefix) {
		return ""
	}

	// Cut the prefix
	afterPrefix := text[len(prefix):]

	// Find the first word after the prefix (split on whitespace)
	fields := strings.Fields(afterPrefix)
	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func BuildCommand(alias string, e *Engine, msg *model.ChatMessage) Command {
	if alias == "say" {
		return command.NewSay(e, msg)
	} else {
		log.Println("Unknown command")
	}

	return nil
}
