package turn

import (
	"context"
	"errors"
	"strings"

	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool/contract"
)

type CommandProseGuard interface{ FindCommand(string) (string, bool) }
type ResponseCorrector interface {
	CorrectUnverifiedAction(context.Context, llm.LlmResponse, []llm.LlmMessage, []contract.Definition, string) (llm.LlmResponse, []llm.LlmMessage, error)
}

type PolicyInput struct {
	Response          llm.LlmResponse
	Messages          []llm.LlmMessage
	Definitions       []contract.Definition
	CommandProseGuard CommandProseGuard
	State             *State
	Prompt            string
	CorrelationID     string
	RequiredFreshTool *string
}

func cloneDefinitions(in []contract.Definition) []contract.Definition {
	out := make([]contract.Definition, len(in))
	for i, d := range in {
		out[i] = contract.Definition{Name: d.Name, Description: d.Description, Parameters: append([]byte(nil), d.Parameters...)}
	}
	return out
}
func clonePolicyInput(in PolicyInput) PolicyInput {
	out := in
	out.Messages = append([]llm.LlmMessage(nil), in.Messages...)
	out.Definitions = cloneDefinitions(in.Definitions)
	if in.RequiredFreshTool != nil {
		v := *in.RequiredFreshTool
		out.RequiredFreshTool = &v
	}
	return out
}
func NewPolicyInput(response llm.LlmResponse, messages []llm.LlmMessage, definitions []contract.Definition, guard CommandProseGuard, state *State, prompt, correlation string, required *string) (PolicyInput, error) {
	if guard == nil || state == nil || messages == nil || definitions == nil {
		return PolicyInput{}, errors.New("policy input dependencies missing")
	}
	if strings.TrimSpace(prompt) == "" || strings.TrimSpace(correlation) == "" {
		return PolicyInput{}, errors.New("policy input identity missing")
	}
	return clonePolicyInput(PolicyInput{response, messages, definitions, guard, state, prompt, correlation, required}), nil
}

type PolicyResult struct {
	Response       llm.LlmResponse
	Messages       []llm.LlmMessage
	CorrectionUsed bool
	Continue       bool
}

func Continue(response llm.LlmResponse, correction bool) PolicyResult {
	return PolicyResult{Response: response, CorrectionUsed: correction, Continue: true}
}
func Stop(response llm.LlmResponse) PolicyResult {
	return PolicyResult{Response: response, Continue: false}
}

type Policy interface {
	Apply(context.Context, PolicyInput) (PolicyResult, error)
}
type PolicyFunc func(context.Context, PolicyInput) (PolicyResult, error)

func (f PolicyFunc) Apply(c context.Context, i PolicyInput) (PolicyResult, error) { return f(c, i) }

type PolicyChain struct {
	policies []Policy
}

func NewPolicyChain(policies []Policy) PolicyChain {
	p := append([]Policy(nil), policies...)
	for _, x := range p {
		if x == nil {
			panic("nil policy")
		}
	}
	return PolicyChain{policies: p}
}
func (c PolicyChain) Apply(ctx context.Context, in PolicyInput) (PolicyResult, error) {
	r := Continue(in.Response, false)
	current := clonePolicyInput(in)
	for _, p := range c.policies {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		current.Response = r.Response
		next, err := p.Apply(ctx, clonePolicyInput(current))
		if err != nil {
			return r, err
		}
		r.Response = next.Response
		r.CorrectionUsed = r.CorrectionUsed || next.CorrectionUsed
		r.Continue = next.Continue
		if next.Messages != nil {
			current.Messages = append([]llm.LlmMessage(nil), next.Messages...)
		}
		if !r.Continue {
			break
		}
	}
	return r, nil
}

type UnverifiedActionPolicy struct {
	guard     CommandProseGuard
	corrector ResponseCorrector
}

func NewUnverifiedActionPolicy(guard CommandProseGuard, corrector ResponseCorrector) (*UnverifiedActionPolicy, error) {
	if guard == nil || corrector == nil {
		return nil, errors.New("unverified-action dependencies missing")
	}
	return &UnverifiedActionPolicy{guard: guard, corrector: corrector}, nil
}
func (p *UnverifiedActionPolicy) Apply(ctx context.Context, in PolicyInput) (PolicyResult, error) {
	if in.State == nil || p == nil || p.guard == nil || p.corrector == nil {
		return PolicyResult{}, errors.New("unverified-action dependencies missing")
	}
	if in.State.UnverifiedActionChecked() {
		return Continue(in.Response, false), nil
	}
	_, recognized := p.guard.FindCommand(in.Response.Content())
	if !in.State.HasAnySuccessfulCommand() || !recognized {
		if err := ctx.Err(); err != nil {
			return PolicyResult{}, err
		}
		r, msgs, err := p.corrector.CorrectUnverifiedAction(ctx, in.Response, in.Messages, in.Definitions, in.CorrelationID)
		if err != nil {
			return PolicyResult{}, err
		}
		in.State.MarkUnverifiedActionChecked()
		result := Continue(r, false)
		result.Messages = append([]llm.LlmMessage(nil), msgs...)
		return result, nil
	}
	return Continue(in.Response, false), nil
}
