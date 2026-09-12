package live

import (
	"context"
	"fmt"
	"zenbot/internal/agent/participation"
	"zenbot/internal/common"
	"zenbot/internal/listener/message"
	"zenbot/internal/model"
)

type RoomParticipation struct {
	Pipeline       *participation.Pipeline
	Snapshot       func(*message.Context) participation.TrustedSnapshot
	AmbientEnabled bool
	AmbientEvery   uint64
	// SemanticCandidate is composed only after the complete typed moderation
	// action prerequisite is available. The adapter owns canonical target use.
	SemanticCandidate func(model.ChatMessage) bool
}

func (p RoomParticipation) Handle(ctx context.Context, c *message.Context) (bool, error) {
	if c == nil || c.Message == nil || c.Engine == nil || p.Pipeline == nil || p.Snapshot == nil {
		return false, fmt.Errorf("agent room participation is not initialized")
	}
	candidate := false
	var target string
	if p.SemanticCandidate != nil && c.Author != nil && !c.Author.IsBot && !c.Author.Isme && p.SemanticCandidate(*c.Message) {
		if c.Author.Name != "" {
			target = c.Author.Name
			candidate = true
		}
	}
	event := participation.Event{Message: *c.Message, BotNick: c.Engine.GetName(), Prefixes: common.CommandPrefixes(c.Engine), AuthorIsBot: c.Author != nil && c.Author.IsBot, AmbientEnabled: p.AmbientEnabled, AmbientEvery: p.AmbientEvery, ModerationCandidate: candidate, ModerationTarget: target}
	out := p.Pipeline.HandleDeferred(event, func() participation.TrustedSnapshot { return p.Snapshot(c) })
	return out.Decision == participation.Claimed, out.Err
}
