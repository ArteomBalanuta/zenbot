package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

// AgentCommandGateway is the narrow command boundary available to the bounded agent action.
type AgentCommandGateway = commandgateway.Gateway
type CommandExecution = commandgateway.Execution

type agentCommandGateway struct{ engine common.Engine }

func NewAgentCommandGateway(engine common.Engine) AgentCommandGateway {
	return agentCommandGateway{engine: engine}
}

var publicAgentCommandAliases = map[string]struct{}{
	"help": {}, "h": {}, "list": {}, "users": {}, "info": {}, "ping": {}, "p": {},
	"weather": {}, "w": {}, "time": {}, "t": {}, "version": {}, "v": {},
}

// agentCaptureEngine is request-scoped: it delegates sends immediately and records only successful sends.
type agentCaptureEngine struct {
	common.Engine
	messages []string
}

func (e *agentCaptureEngine) SendChatMessage(author, message string, whisper bool) (string, error) {
	result, err := e.Engine.SendChatMessage(author, message, whisper)
	if err == nil {
		e.messages = append(e.messages, message)
	}
	return result, err
}

func (g agentCommandGateway) Execute(ctx context.Context, caller api.Context, command, arguments string) (CommandExecution, error) {
	if err := ctx.Err(); err != nil {
		return CommandExecution{}, err
	}
	if g.engine == nil {
		return CommandExecution{}, fmt.Errorf("command gateway is unavailable")
	}
	alias := strings.ToLower(strings.TrimSpace(command))
	if _, ok := publicAgentCommandAliases[alias]; !ok {
		return CommandExecution{}, fmt.Errorf("command is not allowed")
	}
	definition, ok := commandDefinitionFor(alias)
	if !ok || !concretePublicDefinition(definition) {
		return CommandExecution{}, fmt.Errorf("command is unavailable")
	}
	user := g.engine.GetActiveUserByName(caller.Nick())
	if user == nil || !g.engine.IsUserAuthorized(user, &definition.Role) {
		return CommandExecution{}, fmt.Errorf("command is not authorized")
	}
	trip, hash := "", ""
	if v := caller.Trip(); v != nil {
		trip = *v
	}
	if v := caller.Hash(); v != nil {
		hash = *v
	}
	arguments = strings.TrimSpace(arguments)
	text := g.engine.GetPrefix() + alias
	if arguments != "" {
		text += " " + arguments
	}
	message := &model.ChatMessage{Name: caller.Nick(), Trip: trip, Hash: hash, Channel: caller.Room(), Text: text, Whisper: caller.Whisper(), IsWhisper: caller.Whisper()}
	capturing := &agentCaptureEngine{Engine: g.engine}
	status, err := definition.New(capturing, message).Execute(ctx)
	if err != nil {
		return CommandExecution{}, err
	}
	if status != model.SUCCESSFUL {
		return CommandExecution{}, fmt.Errorf("command execution rejected")
	}
	if err := ctx.Err(); err != nil {
		return CommandExecution{}, err
	}
	return CommandExecution{Executed: true, Messages: append([]string(nil), capturing.messages...)}, nil
}

func concretePublicDefinition(d common.CommandDefinition) bool {
	switch d.Canonical {
	case "help", "list", "users", "info", "ping", "weather", "time", "version":
		return d.New != nil
	default:
		return false
	}
}
