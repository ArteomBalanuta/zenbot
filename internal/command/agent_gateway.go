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

// NewResolvingAgentCommandGateway resolves the current master once for each
// request so a persistent agent runtime never retains a retired host.
func NewResolvingAgentCommandGateway(resolve func() common.Engine) AgentCommandGateway {
	return resolvingAgentCommandGateway{resolve: resolve}
}

type resolvingAgentCommandGateway struct{ resolve func() common.Engine }

func (g resolvingAgentCommandGateway) Execute(ctx context.Context, caller api.Context, command, arguments string) (CommandExecution, error) {
	if g.resolve == nil {
		return CommandExecution{}, fmt.Errorf("command gateway is unavailable")
	}
	engine := g.resolve()
	if engine == nil {
		return CommandExecution{}, fmt.Errorf("command gateway is unavailable")
	}
	return NewAgentCommandGateway(engine).Execute(ctx, caller, command, arguments)
}

func (g agentCommandGateway) Execute(ctx context.Context, caller api.Context, command, arguments string) (CommandExecution, error) {
	if err := ctx.Err(); err != nil {
		return CommandExecution{}, err
	}
	if g.engine == nil {
		return CommandExecution{}, fmt.Errorf("command gateway is unavailable")
	}
	alias := strings.ToLower(strings.TrimSpace(command))
	definition, ok := commandDefinitionFor(alias)
	if !ok {
		return CommandExecution{}, fmt.Errorf("command is unavailable")
	}
	approved, ok := AgentCommandDefinition(definition.Canonical)
	if !ok || approved.Canonical != definition.Canonical {
		return CommandExecution{}, fmt.Errorf("command is not allowed")
	}
	if !AgentCommandAuthorized(caller, definition) {
		return CommandExecution{}, fmt.Errorf("command is not authorized")
	}
	if target := caller.ModerationTarget(); target != nil && AgentCommandTargetsUser(definition.Canonical) && !sameCommandTarget(firstCommandArgument(arguments), *target) {
		return CommandExecution{}, fmt.Errorf("moderation action must target the reviewed author")
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
	(&legacyAdapter{engine: capturing, def: definition, msg: message}).audit(ctx, status)
	if err != nil {
		return CommandExecution{}, err
	}
	if status != model.SUCCESSFUL {
		return CommandExecution{}, fmt.Errorf("command execution rejected")
	}
	if err := capturing.awaitSnapshotCompletions(ctx); err != nil {
		return CommandExecution{}, err
	}
	if err := ctx.Err(); err != nil {
		return CommandExecution{}, err
	}
	return CommandExecution{Executed: true, Messages: append([]string(nil), capturing.messages...)}, nil
}

func firstCommandArgument(arguments string) string {
	fields := strings.Fields(arguments)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func sameCommandTarget(argument, expected string) bool {
	return strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(argument), "@"), strings.TrimPrefix(strings.TrimSpace(expected), "@"))
}
