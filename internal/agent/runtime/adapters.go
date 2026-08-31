package runtime

import (
	"errors"

	agentapi "zenbot/internal/agent/api"
)

// ToAPIInvocation is an explicit boundary adapter for callers still using the
// private runtime seam. Runtime timestamps and error envelopes are not Saturn
// components and are intentionally not exported through this mapping.
func ToAPIInvocation(v Invocation) (agentapi.Invocation, error) {
	caps := make([]agentapi.Capability, 0, len(v.Context().Capabilities()))
	for _, c := range v.Context().Capabilities() {
		caps = append(caps, agentapi.Capability(c))
	}
	ctx, err := agentapi.NewContextWithCapabilities(v.Context().Room(), v.Context().Nick(), v.Context().Trip(), v.Context().Hash(), v.Context().Whisper(), v.Context().RoomUsers(), caps, v.Context().ModerationTarget())
	if err != nil {
		return agentapi.Invocation{}, err
	}
	var current any
	if v.CurrentMessageText() != "" {
		current = v.CurrentMessageText()
	}
	return agentapi.NewInvocation(v.RequestID(), ctx, v.Prompt(), agentapi.InvocationMode(v.Mode()), current, v.CommandOriginated())
}

// FromAPIInvocation adapts an exact API invocation to the existing runtime
// execution seam. Nullable text is represented by runtime's legacy empty
// string, the one unavoidable lossy conversion in this compatibility adapter.
func FromAPIInvocation(v agentapi.Invocation) (Invocation, error) {
	c := v.Context()
	caps := make([]Capability, 0, len(c.Capabilities()))
	for _, x := range c.Capabilities() {
		caps = append(caps, Capability(x))
	}
	ctx := NewContextWithCapabilities(c.Room(), c.Nick(), value(c.Trip()), value(c.Hash()), c.Whisper(), c.RoomUsers(), caps, value(c.ModerationTarget()))
	return NewInvocation(v.RequestID(), ctx, v.Prompt(), Mode(v.Mode()), value(v.CurrentMessageText()), v.CommandOriginated()), nil
}

func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// ToAPIResult preserves reply/content and deliberately drops runtime
// ErrorCode: Saturn AgentResult has no error-code component.
func ToAPIResult(v Result) (agentapi.Result, error) {
	if v.ErrorCode() != "" {
		return agentapi.Result{}, errors.New("runtime error result has no Saturn AgentResult representation")
	}
	return agentapi.NewResult(v.CorrelationID(), v.Text(), v.ShouldReply())
}
