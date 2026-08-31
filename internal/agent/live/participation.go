package live

import (
	"context"
	"fmt"
	"zenbot/internal/agent/participation"
	"zenbot/internal/listener/message"
)

type RoomParticipation struct {
	Pipeline       *participation.Pipeline
	Snapshot       func(*message.Context) participation.TrustedSnapshot
	AmbientEnabled bool
	AmbientEvery   uint64
}

func (p RoomParticipation) Handle(ctx context.Context, c *message.Context) (bool, error) {
	if c == nil || c.Message == nil || c.Engine == nil || p.Pipeline == nil || p.Snapshot == nil {
		return false, fmt.Errorf("agent room participation is not initialized")
	}
	out := p.Pipeline.Handle(participation.Event{Message: *c.Message, Snapshot: p.Snapshot(c), BotNick: c.Engine.GetName(), Prefix: c.Engine.GetPrefix(), AuthorIsBot: c.Author != nil && c.Author.IsBot, AmbientEnabled: p.AmbientEnabled, AmbientEvery: p.AmbientEvery, ModerationCandidate: false})
	return out.Decision == participation.Claimed, out.Err
}
