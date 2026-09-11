package command

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/repository"
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
		return CommandExecution{Status: commandgateway.OutcomeRejected}, fmt.Errorf("command gateway is unavailable")
	}
	engine := g.resolve()
	if engine == nil {
		return CommandExecution{Status: commandgateway.OutcomeRejected}, fmt.Errorf("command gateway is unavailable")
	}
	return NewAgentCommandGateway(engine).Execute(ctx, caller, command, arguments)
}

func (g agentCommandGateway) Execute(ctx context.Context, caller api.Context, command, arguments string) (result CommandExecution, resultErr error) {
	if err := ctx.Err(); err != nil {
		return CommandExecution{Status: commandgateway.OutcomeRejected}, err
	}
	if g.engine == nil {
		return CommandExecution{Status: commandgateway.OutcomeRejected}, fmt.Errorf("command gateway is unavailable")
	}
	alias := strings.ToLower(strings.TrimSpace(command))
	definition, ok := commandDefinitionFor(alias)
	if !ok {
		return CommandExecution{Status: commandgateway.OutcomeRejected}, fmt.Errorf("command is unavailable")
	}
	approved, ok := AgentCommandDefinition(definition.Canonical)
	if !ok || approved.Canonical != definition.Canonical {
		return CommandExecution{Status: commandgateway.OutcomeRejected}, fmt.Errorf("command is not allowed")
	}
	if !AgentCommandAuthorized(caller, definition) {
		return CommandExecution{Status: commandgateway.OutcomeRejected}, fmt.Errorf("command is not authorized")
	}
	if target := caller.ModerationTarget(); target != nil && AgentCommandTargetsUser(definition.Canonical) && !sameCommandTarget(firstCommandArgument(arguments), *target) {
		return CommandExecution{Status: commandgateway.OutcomeRejected}, fmt.Errorf("moderation action must target the reviewed author")
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
	capturing := &agentCaptureEngine{Engine: g.engine, invocationWhisper: caller.Whisper()}
	// Effects may already exist when a later command operation or audit fails.
	// Preserve receipts on every return, including panics after delivery.
	defer func() {
		if recover() != nil {
			result.Status = commandgateway.OutcomeUnknown
			resultErr = nil
		}
		result.Messages = append([]string(nil), capturing.messages...)
		if result.Status == commandgateway.OutcomeSucceeded {
			result.Data = append(result.Data[:0:0], capturing.data...)
		}
		if capturing.actionCount > 0 {
			result.EffectsCommitted = true
			result.Action = &commandgateway.ActionReceipt{Count: capturing.actionCount}
		}
		if capturing.deliveryCount > 0 {
			result.Delivery = &commandgateway.DeliveryReceipt{Count: capturing.deliveryCount}
		}
	}()
	status, err := definition.New(capturing, message).Execute(ctx)
	(&legacyAdapter{engine: capturing, def: definition, msg: message}).audit(ctx, status)
	if len(capturing.snapshotCompletions) > 0 {
		_ = capturing.awaitSnapshotCompletions(ctx)
		return CommandExecution{Status: capturing.snapshotStatus}, nil
	}
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return CommandExecution{Status: commandgateway.OutcomeNotFound}, nil
		}
		return CommandExecution{Status: commandgateway.OutcomeUnknown}, nil
	}
	if status != model.SUCCESSFUL {
		return CommandExecution{Status: commandgateway.OutcomeRejected}, nil
	}
	return CommandExecution{Status: commandgateway.OutcomeSucceeded}, nil
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
