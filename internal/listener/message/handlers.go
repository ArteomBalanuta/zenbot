package message

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/service"
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

func (AuditChatMessage) Handle(_ context.Context, c *Context) (bool, error) {
	_, err := c.Engine.LogMessage(c.Message.Trip, c.Message.Name, c.Message.Hash, c.Message.Text, c.Engine.GetChannel())
	return true, err
}

type IgnoreBotMessage struct{}

func (IgnoreBotMessage) Handle(_ context.Context, c *Context) (bool, error) {
	return !strings.EqualFold(c.Engine.GetName(), c.Message.Name), nil
}

type RelayAgentMessage struct{}

func (RelayAgentMessage) Handle(_ context.Context, _ *Context) (bool, error) { return true, nil }

type LogChatMessage struct{}

func (LogChatMessage) Handle(_ context.Context, c *Context) (bool, error) {
	log.Printf("hash: %s, trip: %s, nick: %s, message: %s", c.Message.Hash, c.Message.Trip, c.Message.Name, c.Message.Text)
	return true, nil
}

type DeliverPendingMail struct{}

func (DeliverPendingMail) Handle(_ context.Context, c *Context) (bool, error) {
	b := serviceBundle(c.Engine)
	if b == nil || b.Mail == nil {
		return true, nil
	}
	mails, err := b.Mail.Pending(c.Message.Name, c.Message.Trip)
	if err != nil {
		return true, err
	}
	var whisper, public []model.Mail
	for _, m := range mails {
		if m.IsWhisper {
			whisper = append(whisper, m)
		} else {
			public = append(public, m)
		}
	}
	format := func(ms []model.Mail) string {
		var out string
		for _, m := range ms {
			out += time.UnixMilli(m.CreatedOn).UTC().Format(time.RFC1123) + ".\\n" + m.Owner + ": " + m.Message + "\\n &nbsp; \\n"
		}
		return out
	}
	if len(whisper) > 0 {
		_, err = c.Engine.SendChatMessage(c.Message.Name, " new mail: \\n "+format(whisper), true)
		if err != nil {
			return true, err
		}
	}
	if len(public) > 0 {
		_, err = c.Engine.SendChatMessage(c.Message.Name, " new mail: \\n "+format(public), false)
		if err != nil {
			return true, err
		}
	}
	for _, m := range mails {
		if err = b.Mail.MarkDelivered(m.ID); err != nil {
			return true, err
		}
	}
	return true, nil
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

func (YoutubePreview) Handle(_ context.Context, _ *Context) (bool, error) { return true, nil }

type CernEasterEgg struct{}

func (CernEasterEgg) Handle(_ context.Context, c *Context) (bool, error) {
	if strings.Contains(strings.ToLower(c.Message.Text), "has cern ended the universe") {
		_, err := c.Engine.SendChatMessage("", "no", false)
		return true, err
	}
	return true, nil
}

type AgentParticipation struct{}

func (AgentParticipation) Handle(_ context.Context, _ *Context) (bool, error) { return true, nil }

type DispatchUserCommand struct{}

func (DispatchUserCommand) Handle(_ context.Context, c *Context) (bool, error) {
	text := strings.TrimSpace(c.Message.Text)
	if !strings.HasPrefix(text, c.Engine.GetPrefix()) {
		return false, nil
	}
	fields := strings.Fields(strings.TrimPrefix(text, c.Engine.GetPrefix()))
	if len(fields) == 0 {
		return false, nil
	}
	cmd := common.BuildCommand(fields[0], c.Engine, c.Message)
	if cmd == nil {
		return false, nil
	}
	if c.Author == nil || !c.Engine.IsUserAuthorized(c.Author, cmd.GetRole()) {
		if c.Author != nil {
			_, _ = c.Engine.SendChatMessage(c.Author.Name, fmt.Sprintf(" you are not authorized to run: %s command.", fields[0]), c.Message.IsWhisper)
		}
		return false, nil
	}
	cmd.Execute()
	return false, nil
}
func DefaultChain() *Chain {
	return NewChain(ResolveUserMetadata{}, AuditChatMessage{}, IgnoreBotMessage{}, RelayAgentMessage{}, LogChatMessage{}, DeliverPendingMail{}, UpdateAfkState{}, YoutubePreview{}, CernEasterEgg{}, AgentParticipation{}, DispatchUserCommand{})
}
