package message

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/profiling"
	"zenbot/internal/relay"
	"zenbot/internal/service"
	"zenbot/internal/util"
)

type ResolveUserMetadata struct{}

func (ResolveUserMetadata) Handle(_ context.Context, c *Context) (bool, error) {
	for u := range *c.Engine.GetActiveUsers() {
		if strings.EqualFold(u.Name, c.Message.Name) {
			c.Author = u
			c.Message.Hash = u.Hash
			break
		}
	}
	return true, nil
}

type AuditChatMessage struct{}

func (AuditChatMessage) Handle(ctx context.Context, c *Context) (bool, error) {
	whisper := c.Message.IsWhisper || c.Message.Whisper || c.Message.Type == "whisper"
	if auditor, ok := c.Engine.(common.MessageRecordAuditor); ok {
		visibility := "PUBLIC"
		if whisper {
			visibility = "WHISPER"
		}
		_, err := auditor.LogMessageRecord(ctx, model.MessageRecord{
			Trip: c.Message.Trip, Name: c.Message.Name, Hash: c.Message.Hash,
			Message: c.Message.Text, Channel: c.Engine.GetChannel(),
			Visibility: visibility, CreatedOnMillis: time.Now().UnixMilli(),
		})
		return true, err
	}
	if whisper {
		return false, fmt.Errorf("whisper audit requires visibility-aware storage")
	}
	_, err := c.Engine.LogMessage(c.Message.Trip, c.Message.Name, c.Message.Hash, c.Message.Text, c.Engine.GetChannel())
	return true, err
}

type IgnoreBotMessage struct{}

func (IgnoreBotMessage) Handle(_ context.Context, c *Context) (bool, error) {
	return !strings.EqualFold(c.Engine.GetName(), c.Message.Name), nil
}

type RelayAgentMessage struct{}

func (RelayAgentMessage) Handle(ctx context.Context, c *Context) (bool, error) {
	agent, ok := c.Engine.(interface {
		EngineType() model.EngineType
		relay.AgentHostRef
	})
	if !ok || agent.EngineType() != model.AGENT {
		return true, nil
	}
	host := agent.HostRelay()
	if host == nil {
		log.Printf("agent relay host missing")
		return false, nil
	}
	if err := host.RelayAgentMessage(ctx, c.Message.Name, c.Message.Text); err != nil {
		return false, err
	}
	return false, nil
}

type LogChatMessage struct{}

func (LogChatMessage) Handle(_ context.Context, c *Context) (bool, error) {
	log.Printf("hash: %s, trip: %s, nick: %s, message: %s", c.Message.Hash, c.Message.Trip, c.Message.Name, c.Message.Text)
	return true, nil
}

type DeliverPendingMail struct{}

func (DeliverPendingMail) Handle(ctx context.Context, c *Context) (bool, error) {
	b := serviceBundle(c.Engine)
	if b == nil || b.Mail == nil {
		return true, nil
	}
	err := b.Mail.DeliverPending(ctx, c.Message.Name, c.Message.Trip, func(ctx context.Context, m model.Mail) error {
		if err := ctx.Err(); err != nil {
			return errors.Join(service.ErrMailSendNotStarted, err)
		}
		text := "Mail from " + m.Owner + " · " + util.CompactTime(time.UnixMilli(m.CreatedOn), time.Now()) + "\n" + m.Message
		_, err := c.Engine.SendChatMessage(c.Message.Name, text, m.IsWhisper)
		return err
	})
	return true, err
}

func serviceBundle(e common.Engine) *service.Bundle {
	if x, ok := e.(interface{ ServiceBundle() *service.Bundle }); ok {
		return x.ServiceBundle()
	}
	return nil
}

type UpdateAfkState struct{}

func (UpdateAfkState) Handle(_ context.Context, c *Context) (bool, error) {
	if c.Author != nil {
		c.Engine.RemoveIfAfk(c.Author)
		c.Engine.NotifyAfkIfMentioned(c.Message)
	}
	return true, nil
}

type YoutubePreview struct{}

func (YoutubePreview) Handle(ctx context.Context, c *Context) (bool, error) {
	bundle := serviceBundle(c.Engine)
	if bundle == nil || bundle.YouTube == nil {
		return true, nil
	}
	preview, found, err := bundle.YouTube.Preview(ctx, c.Message.Text)
	if err != nil {
		log.Printf("could not generate YouTube preview: %v", err)
		return true, nil
	}
	if found {
		_, err = c.Engine.SendChatMessage(c.Message.Name, preview, false)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

type CernEasterEgg struct{}

func (CernEasterEgg) Handle(_ context.Context, c *Context) (bool, error) {
	if strings.Contains(strings.ToLower(c.Message.Text), "has cern ended the universe") {
		_, err := c.Engine.SendChatMessage("", "no", false)
		return true, err
	}
	return true, nil
}

type Participation interface {
	Handle(context.Context, *Context) (bool, error)
}
type PassParticipation struct{}

func (PassParticipation) Handle(context.Context, *Context) (bool, error) { return false, nil }

type AgentParticipation struct{ Participation Participation }

func (h AgentParticipation) Handle(ctx context.Context, c *Context) (bool, error) {
	p := h.Participation
	if p == nil {
		p = PassParticipation{}
	}
	claimed, err := p.Handle(ctx, c)
	if err != nil {
		return false, err
	}
	return !claimed, nil
}

type DispatchUserCommand struct{}

type canonicalCommand interface {
	CanonicalName() string
}

func isCommandAuthorized(engine common.Engine, cmd common.Command, author *model.User) bool {
	return common.IsCommandAuthorized(engine, cmd, author)
}

func (DispatchUserCommand) Handle(ctx context.Context, c *Context) (bool, error) {
	text := strings.TrimSpace(c.Message.Text)
	prefix := common.MatchCommandPrefix(text, common.CommandPrefixes(c.Engine))
	if prefix == "" {
		return false, nil
	}
	fields := strings.Fields(strings.TrimPrefix(text, prefix))
	if len(fields) == 0 {
		return false, nil
	}
	lookupDone := profiling.Measure(ctx, "command.lookup")
	cmd := common.BuildCommand(fields[0], c.Engine, c.Message)
	lookupDone()
	if cmd == nil {
		return false, nil
	}
	if named, ok := cmd.(canonicalCommand); ok {
		profiling.SetCommandName(ctx, named.CanonicalName())
	}
	authorizationDone := profiling.Measure(ctx, "command.authorization")
	authorized := c.Author != nil && isCommandAuthorized(c.Engine, cmd, c.Author)
	authorizationDone()
	if !authorized {
		if c.Author != nil {
			replyDone := profiling.Measure(ctx, "command.unauthorized_reply")
			_, err := c.Engine.SendChatMessage(c.Author.Name, fmt.Sprintf(" you are not authorized to run: %s command.", fields[0]), c.Message.IsWhisper)
			replyDone()
			return false, err
		}
		return false, nil
	}
	_, err := common.InvokeCommand(ctx, c.Engine, cmd)
	return false, err
}
func DefaultChain() *Chain {
	return DefaultChainWithParticipation(PassParticipation{})
}
func DefaultChainWithParticipation(p Participation) *Chain {
	if p == nil {
		p = PassParticipation{}
	}
	return NewChain(ResolveUserMetadata{}, AuditChatMessage{}, IgnoreBotMessage{}, RelayAgentMessage{}, LogChatMessage{}, DeliverPendingMail{}, UpdateAfkState{}, YoutubePreview{}, CernEasterEgg{}, AgentParticipation{Participation: p}, DispatchUserCommand{})
}
