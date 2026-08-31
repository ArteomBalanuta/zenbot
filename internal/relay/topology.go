// Package relay owns the narrow, one-way contract between an AGENT child
// engine and its permanent host's outbound chat transport.
package relay

import (
	"context"
	"errors"
)

// HostRelay delivers an AGENT child's inbound chat through its host.
type HostRelay interface {
	RelayAgentMessage(ctx context.Context, author, text string) error
}

// AgentHostRef exposes the immutable host relay installed when an AGENT engine
// is created. It deliberately has no setter.
type AgentHostRef interface {
	HostRelay() HostRelay
}

// ChatSender is the existing host public-chat capability needed for relaying.
type ChatSender interface {
	SendChatMessage(author, message string, whisper bool) (string, error)
}

type hostRelay struct {
	sender ChatSender
}

// NewHost adapts a permanent host's existing direct public-chat transport.
// It intentionally leaves text unchanged: SendChatMessage performs the one
// required JSON serialization step for the wire payload.
func NewHost(sender ChatSender) HostRelay {
	return hostRelay{sender: sender}
}

func (h hostRelay) RelayAgentMessage(ctx context.Context, author, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if h.sender == nil {
		return errors.New("agent relay host sender is nil")
	}
	_, err := h.sender.SendChatMessage("", author+": "+text, false)
	return err
}
