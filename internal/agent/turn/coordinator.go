package turn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
)

type FreshExecutor interface {
	Execute(context.Context, api.Context, execution.Call) contract.Result
}
type FreshProcessInput struct {
	Response                   llm.LlmResponse
	Messages                   []llm.LlmMessage
	RequiredTool, RequiredNick string
	Context                    api.Context
	State                      *State
	Definitions                []contract.Definition
}
type DefinitionProvider interface {
	DefinitionsFor([]contract.Definition, string) ([]contract.Definition, error)
}
type ToolResultRenderer interface {
	Render(api.Context, llm.LlmToolCall, contract.Result) string
}
type FreshProcessResult struct {
	Response  llm.LlmResponse
	Messages  []llm.LlmMessage
	Corrected bool
}
type FreshDataCoordinator struct {
	Client      llm.LlmClient
	Executor    FreshExecutor
	Definitions DefinitionProvider
	Renderer    ToolResultRenderer
}

func (c FreshDataCoordinator) Process(ctx context.Context, in FreshProcessInput) (FreshProcessResult, error) {
	if in.State == nil || (in.RequiredTool != "" && c.Client == nil) || (in.RequiredTool != "" && c.Executor == nil) {
		return FreshProcessResult{}, errors.New("fresh coordinator dependencies missing")
	}
	if in.RequiredTool == "" || in.State.HasSuccessfulTool(in.RequiredTool) {
		return FreshProcessResult{in.Response, append([]llm.LlmMessage(nil), in.Messages...), false}, nil
	}
	calls := in.Response.ToolCalls()
	var call llm.LlmToolCall
	if len(calls) == 1 && MatchesFreshCall(calls[0], in.RequiredTool, in.RequiredNick) {
		call = calls[0]
	}
	if call.Name() == "" {
		if in.RequiredTool == UserMessageHistory {
			call = llm.NewLlmToolCall("fresh-history-1", UserMessageHistory, map[string]any{"nick": in.RequiredNick})
		} else {
			if !in.State.ToolsEnabled() {
				return FreshProcessResult{}, errors.New("required fresh-data tool unavailable after tool-call budget exhaustion")
			}
			if in.State.FreshnessCorrectionUsed() {
				return FreshProcessResult{}, errors.New("required fresh-data tool call missing")
			}
			if err := ctx.Err(); err != nil {
				return FreshProcessResult{}, err
			}
			msgs := append([]llm.LlmMessage(nil), in.Messages...)
			msgs = append(msgs,
				llm.NewLlmMessage("assistant", in.Response.Content(), nil, ""),
				llm.NewLlmMessage("user", fmt.Sprintf("The newest request requires fresh data from the `%s` tool in this invocation. Prior conversation\nmemory and prior summaries do not satisfy this requirement. Call that tool now with arguments\nresolved from the newest request and shared context. Do not answer the user before the tool result.", in.RequiredTool), nil, ""))
			selected := in.Definitions
			if c.Definitions != nil {
				var err error
				selected, err = c.Definitions.DefinitionsFor(selected, in.RequiredTool)
				if err != nil {
					return FreshProcessResult{}, err
				}
			}
			defs := make([]any, len(selected))
			for i := range selected {
				defs[i] = selected[i]
			}
			next, err := c.Client.Complete(ctx, llm.NewLlmRequest(msgs, defs, false, nil, nil))
			if err != nil {
				return FreshProcessResult{}, err
			}
			if !exactFreshResponse(next, in.RequiredTool, in.RequiredNick) {
				return FreshProcessResult{}, errors.New("required fresh-data tool call missing")
			}
			in.State.MarkFreshnessCorrectionUsed()
			call = next.ToolCalls()[0]
			in.Response = next
			in.Messages = msgs
		}
	}
	if !in.State.ReserveToolCalls(1) {
		in.State.DisableTools()
		return FreshProcessResult{}, errors.New("tool budget exhausted before fresh lookup")
	}
	if err := in.State.MarkToolAttempted(1); err != nil {
		return FreshProcessResult{}, err
	}
	r := c.Executor.Execute(ctx, in.Context, execution.FromLLM(call))
	if r.IsError {
		_ = in.State.RecordToolFailure()
		return FreshProcessResult{}, fmt.Errorf("required fresh-data tool failed: %s", in.RequiredTool)
	}
	_ = in.State.RecordToolSuccess()
	in.State.RecordSuccessfulTool(in.RequiredTool)
	if err := in.State.RecordSuccessfulToolResult(r); err != nil {
		return FreshProcessResult{}, err
	}
	msgs := append([]llm.LlmMessage(nil), in.Messages...)
	toolContent := r.Content
	if c.Renderer != nil {
		toolContent = c.Renderer.Render(in.Context, call, r)
	}
	msgs = append(msgs, llm.NewLlmMessage("assistant", in.Response.Content(), []llm.LlmToolCall{call}, ""), llm.NewLlmMessage("tool", toolContent, nil, call.ID()))
	next, err := c.Client.Complete(ctx, llm.NewLlmRequest(msgs, nil, false, nil, nil))
	if err != nil {
		return FreshProcessResult{}, err
	}
	if err := (FinalValidator{}).ValidateWithHistory(next, msgs, in.RequiredTool, in.State.SuccessfulToolResults()); err != nil {
		if in.State.FreshSynthesisCorrectionUsed() {
			return FreshProcessResult{}, err
		}
		if e := ctx.Err(); e != nil {
			return FreshProcessResult{}, e
		}
		correctionMsgs := append(append([]llm.LlmMessage(nil), msgs...), llm.NewLlmMessage("assistant", next.Content(), next.ToolCalls(), ""), llm.NewLlmMessage("user", "router-fresh-synthesis-correction", nil, ""))
		corrected, e := c.Client.Complete(ctx, llm.NewLlmRequest(correctionMsgs, nil, false, nil, nil))
		if e != nil {
			return FreshProcessResult{}, e
		}
		in.State.MarkFreshSynthesisCorrectionUsed()
		if e = (FinalValidator{}).ValidateWithHistory(corrected, correctionMsgs, in.RequiredTool, in.State.SuccessfulToolResults()); e != nil {
			return FreshProcessResult{}, e
		}
		next, msgs = corrected, correctionMsgs
	}
	return FreshProcessResult{next, msgs, true}, nil
}

type FinalValidator struct{}

func (FinalValidator) Validate(response llm.LlmResponse, state *State, requiredTool string) error {
	if state == nil {
		return errors.New("required fresh-data evidence missing")
	}
	return (FinalValidator{}).ValidateWithHistory(response, nil, requiredTool, state.SuccessfulToolResults())
}

func exactFreshResponse(r llm.LlmResponse, tool, nick string) bool {
	calls := r.ToolCalls()
	if len(calls) != 1 || !MatchesFreshCall(calls[0], tool, nick) {
		return false
	}
	raw := strings.TrimSpace(calls[0].RawArguments())
	if raw == "" {
		return false
	}
	var obj map[string]any
	if json.Unmarshal([]byte(raw), &obj) != nil || obj == nil {
		return false
	}
	return true
}
func (FinalValidator) ValidateWithHistory(response llm.LlmResponse, history []llm.LlmMessage, requiredTool string, successful []contract.Result) error {
	if requiredTool == "" {
		return nil
	}
	found := false
	for _, r := range successful {
		if !r.IsError && r.ToolName == requiredTool && strings.TrimSpace(r.Content) != "" {
			found = true
			break
		}
	}
	if !found {
		return errors.New("required fresh-data evidence missing")
	}
	if len(response.ToolCalls()) != 0 {
		return errors.New("fresh synthesis contains tool calls")
	}
	if strings.TrimSpace(response.Content()) == "" {
		return errors.New("fresh history synthesis is empty")
	}
	if strings.TrimSpace(LatestConversationAssistant(history)) == strings.TrimSpace(response.Content()) {
		return errors.New("fresh synthesis repeats previous assistant")
	}
	return nil
}
