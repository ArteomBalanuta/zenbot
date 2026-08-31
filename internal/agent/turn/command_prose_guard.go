package turn

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"

	"zenbot/internal/agent/tool/contract"
)

var (
	backtickFence = regexp.MustCompile("(?ms)^[ \\t]{0,3}`{3,}[^\\r\\n]*\\r?\\n(.*?)^[ \\t]{0,3}`{3,}[ \\t]*$")
	tildeFence    = regexp.MustCompile("(?ms)^[ \\t]{0,3}~{3,}[^\\r\\n]*\\r?\\n(.*?)^[ \\t]{0,3}~{3,}[ \\t]*$")
)

// ConcreteCommandProseGuard derives command aliases only from advertised run_command definitions.
type ConcreteCommandProseGuard struct{ aliases map[string]struct{} }

func NewCommandProseGuard(definitions []contract.Definition) *ConcreteCommandProseGuard {
	aliases := map[string]struct{}{}
	for _, definition := range definitions {
		if definition.Name != "run_command" {
			continue
		}
		var schema struct {
			Properties map[string]struct {
				Enum []any `json:"enum"`
			} `json:"properties"`
		}
		if json.Unmarshal(definition.Parameters, &schema) != nil || schema.Properties == nil {
			continue
		}
		command, ok := schema.Properties["command"]
		if !ok || len(command.Enum) == 0 {
			continue
		}
		valid := true
		for _, value := range command.Enum {
			alias, ok := value.(string)
			if !ok || strings.TrimSpace(alias) == "" {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		for _, value := range command.Enum {
			aliases[strings.ToLower(value.(string))] = struct{}{}
		}
	}
	return &ConcreteCommandProseGuard{aliases: aliases}
}

func (g *ConcreteCommandProseGuard) FindCommand(content string) (string, bool) {
	if g == nil || strings.TrimSpace(content) == "" {
		return "", false
	}
	for _, match := range backtickFence.FindAllStringSubmatch(content, -1) {
		if command, ok := g.commandAtStart(match[1]); ok {
			return command, true
		}
	}
	for _, match := range tildeFence.FindAllStringSubmatch(content, -1) {
		if command, ok := g.commandAtStart(match[1]); ok {
			return command, true
		}
	}
	return g.findInline(content)
}

func (g *ConcreteCommandProseGuard) findInline(content string) (string, bool) {
	for index := 0; index < len(content); {
		start := strings.IndexByte(content[index:], '`')
		if start < 0 {
			break
		}
		start += index
		if start > 0 && content[start-1] == '`' {
			index = start + 1
			continue
		}
		width := 1
		for start+width < len(content) && content[start+width] == '`' {
			width++
		}
		needle := strings.Repeat("`", width)
		endOffset := strings.Index(content[start+width:], needle)
		if endOffset < 0 {
			index = start + width
			continue
		}
		end := start + width + endOffset
		if end+width < len(content) && content[end+width] == '`' {
			index = end + width
			continue
		}
		snippet := content[start+width : end]
		if !strings.ContainsAny(snippet, "\r\n") {
			if command, ok := g.commandAtStart(snippet); ok {
				return command, true
			}
		}
		index = end + width
	}
	return "", false
}

func (g *ConcreteCommandProseGuard) MatchesRunCommand(expected, name string, arguments map[string]any) bool {
	if g == nil || name != "run_command" || !g.allowed(expected) || len(arguments) == 0 || len(arguments) > 2 {
		return false
	}
	command, ok := arguments["command"].(string)
	if !ok || command != expected {
		return false
	}
	if raw, exists := arguments["arguments"]; exists {
		if _, ok := raw.(string); !ok {
			return false
		}
	}
	for key := range arguments {
		if key != "command" && key != "arguments" {
			return false
		}
	}
	return true
}

func (g *ConcreteCommandProseGuard) commandAtStart(snippet string) (string, bool) {
	normalized := strings.TrimLeftFunc(snippet, unicode.IsSpace)
	if normalized == "" {
		return "", false
	}
	first, size := utf8Rune(normalized)
	if !unicode.IsLetter(first) && !unicode.IsDigit(first) {
		normalized = strings.TrimLeftFunc(normalized[size:], unicode.IsSpace)
	}
	if normalized == "" {
		return "", false
	}
	end := strings.IndexFunc(normalized, unicode.IsSpace)
	if end < 0 {
		end = len(normalized)
	}
	command := strings.ToLower(normalized[:end])
	return command, g.allowed(command)
}

func (g *ConcreteCommandProseGuard) allowed(command string) bool {
	_, ok := g.aliases[strings.ToLower(command)]
	return ok
}

func utf8Rune(s string) (rune, int) {
	for _, r := range s {
		return r, len(string(r))
	}
	return 0, 0
}

var _ CommandProseGuard = (*ConcreteCommandProseGuard)(nil)
