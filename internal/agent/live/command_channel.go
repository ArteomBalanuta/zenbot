package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
	"zenbot/internal/agent/turn"
)

const respondWithoutCommand = "respond_without_command"

// commandChannel is a private, one-shot correction protocol for rendered commands.
type commandChannel struct {
	client        llm.LlmClient
	registry      *tool.Registry
	allowed       []string
	runDefinition any
	guard         *turn.ConcreteCommandProseGuard
}

func newCommandChannel(agent api.Context, client llm.LlmClient, registry *tool.Registry, allowed []string, definitions []any) (*commandChannel, error) {
	if client == nil || registry == nil {
		return nil, errors.New("command channel is not initialized")
	}
	registered, ok := registry.Lookup("run_command")
	if !ok {
		return nil, errors.New("run command is unavailable")
	}
	descriptor, err := registered.Descriptor(agent)
	if err != nil || descriptor.Name() != "run_command" {
		return nil, errors.New("run command descriptor is invalid")
	}
	var advertised any
	var exposed contract.Definition
	for _, definition := range definitions {
		candidate, ok := exposedRunCommandDefinition(definition)
		if ok {
			advertised = definition
			exposed = candidate
			break
		}
	}
	if advertised == nil {
		return nil, errors.New("run command definition is unavailable")
	}
	return &commandChannel{client: client, registry: registry, allowed: append([]string(nil), allowed...), runDefinition: advertised, guard: turn.NewCommandProseGuard([]contract.Definition{exposed})}, nil
}

func (c *commandChannel) correct(ctx context.Context, inv runtime.Invocation, agent api.Context, prepared []llm.LlmMessage, first llm.LlmResponse, expected string, state *turn.State) (Completion, error) {
	if c == nil || state == nil || strings.TrimSpace(expected) == "" || state.CommandCorrectionUsed() || !state.AdvanceStep() {
		return Completion{}, errors.New("command correction is unavailable")
	}
	state.MarkCommandCorrectionUsed()
	messages := append([]llm.LlmMessage(nil), prepared...)
	messages = append(messages, llm.NewLlmMessage("assistant", first.Content(), nil, ""))
	messages = append(messages, llm.NewLlmMessage("user", "The rendered command was not executed. Return exactly one matching run_command call for the rendered command, or respond_without_command with exactly one non-command response string.", nil, ""))
	second, err := c.client.Complete(ctx, llm.NewLlmRequest(messages, []any{c.runDefinition, responseWithoutCommandDefinition()}, false, nil, nil))
	if err != nil {
		return Completion{}, fmt.Errorf("complete command correction: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Completion{}, err
	}
	if second.FinishReason() == "length" || len(second.ToolCalls()) != 1 {
		return Completion{}, errors.New("invalid command correction")
	}
	call := second.ToolCalls()[0]
	if strings.TrimSpace(call.ID()) == "" {
		return Completion{}, errors.New("invalid command correction")
	}
	if c.guard.MatchesRunCommand(expected, call.Name(), call.Arguments()) {
		return c.execute(ctx, agent, execution.FromLLM(call), state)
	}
	if call.Name() != respondWithoutCommand {
		return Completion{}, errors.New("invalid command correction")
	}
	response, ok := correctedResponse(call.Arguments())
	if !ok || c.guardHasCommand(response) {
		return Completion{}, errors.New("invalid non-command correction")
	}
	return Completion{Response: llm.NewLlmResponse(response, nil, "stop")}, nil
}

func (c *commandChannel) execute(ctx context.Context, agent api.Context, call execution.Call, state *turn.State) (Completion, error) {
	registered, ok := c.registry.Lookup(call.Name)
	if !ok || !c.registry.Allowed(call.Name) {
		return Completion{}, errors.New("unknown bounded tool call")
	}
	descriptor, err := registered.Descriptor(agent)
	if err != nil || contract.ValidateArguments(descriptor.Parameters(), call.Arguments) != nil || !state.ReserveToolCalls(1) || state.MarkToolAttempted(1) != nil {
		return Completion{}, errors.New("invalid bounded tool arguments")
	}
	limits := make(map[string]int, len(c.allowed))
	for _, name := range c.allowed {
		limits[name] = 1
	}
	result := (&execution.Executor{Registry: c.registry, Ledger: execution.NewLedger(limits, 1)}).Execute(ctx, agent, call)
	if err := ctx.Err(); err != nil {
		return Completion{}, err
	}
	if result.IsError {
		_ = state.RecordToolFailure()
		return Completion{}, errors.New("run command failed")
	}
	if err := state.RecordToolSuccess(); err != nil {
		return Completion{}, err
	}
	return Completion{SuppressReply: true}, nil
}

func (c *commandChannel) guardHasCommand(content string) bool {
	_, found := c.guard.FindCommand(content)
	return found
}

func responseWithoutCommandDefinition() any {
	return map[string]any{"type": "function", "function": map[string]any{"name": respondWithoutCommand, "description": "Return a non-command response.", "parameters": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"response": map[string]any{"type": "string"}}, "required": []string{"response"}}}}
}

func correctedResponse(arguments map[string]any) (string, bool) {
	if len(arguments) != 1 {
		return "", false
	}
	response, ok := arguments["response"].(string)
	if !ok || strings.TrimSpace(response) == "" {
		return "", false
	}
	return strings.TrimSpace(response), true
}

// exposedRunCommandDefinition retains the exact schema that will be sent to
// the provider. Command prose recognition must never derive aliases from a
// parallel catalog or merely from the registered implementation.
func exposedRunCommandDefinition(raw any) (contract.Definition, bool) {
	definition, ok := raw.(map[string]any)
	if !ok {
		return contract.Definition{}, false
	}
	function, ok := definition["function"].(map[string]any)
	if !ok {
		return contract.Definition{}, false
	}
	name, ok := function["name"].(string)
	if !ok || name != "run_command" {
		return contract.Definition{}, false
	}
	parameters, ok := function["parameters"]
	if !ok {
		return contract.Definition{}, false
	}
	encoded, err := json.Marshal(parameters)
	if err != nil {
		return contract.Definition{}, false
	}
	return contract.Definition{Name: name, Parameters: encoded}, true
}
