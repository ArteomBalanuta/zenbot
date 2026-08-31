package info

import (
	"context"
	"fmt"
	"strings"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type CaptureBanishedUser struct{}

func (CaptureBanishedUser) Handle(_ context.Context, c *Context) (bool, error) {
	if i := strings.Index(c.Message.Text, " was banished to ?"); i >= 0 {
		c.Engine.SetLastKickedUser(c.Message.Text[:i])
		c.Engine.SetLastKickedChannel(c.Message.Text[i+len(" was banished to ?"):])
	}
	return true, nil
}

type IgnoreSelfWhisperInfo struct{}

func (IgnoreSelfWhisperInfo) Handle(_ context.Context, c *Context) (bool, error) {
	return !strings.Contains(c.Message.Text, "You whispered"), nil
}

type RenameAfkUsers struct{}

func (RenameAfkUsers) Handle(_ context.Context, c *Context) (bool, error) {
	parts := strings.SplitN(c.Message.Text, " is now ", 2)
	if len(parts) == 2 && c.Engine.GetName() == parts[0] {
		c.Engine.SetName(parts[1])
	}
	return true, nil
}

type ConvertWhisperToChatMessage struct{}

func (ConvertWhisperToChatMessage) Handle(_ context.Context, c *Context) (bool, error) {
	if c.Engine == nil || c.Message == nil {
		return false, nil
	}
	from := fmt.Sprint(c.Message.From)
	if from == "<nil>" || from == "" {
		return false, nil
	}
	marker := from + " whispered: "
	i := strings.Index(c.Message.Text, marker)
	if i < 0 {
		return false, nil
	}
	var u *model.User
	for x := range *c.Engine.GetActiveUsers() {
		if strings.EqualFold(x.Name, from) {
			u = x
			break
		}
	}
	if u == nil {
		return false, nil
	}
	c.ChatMessage = &model.ChatMessage{IsWhisper: true, Name: u.Name, Trip: u.Trip, Hash: u.Hash, Text: c.Message.Text[i+len(marker):], Cmd: ""}
	return true, nil
}

type AuditWhisperCommand struct{}

func (AuditWhisperCommand) Handle(_ context.Context, c *Context) (bool, error) {
	if c.ChatMessage == nil {
		return false, nil
	}
	_, err := c.Engine.LogMessage(c.ChatMessage.Trip, c.ChatMessage.Name, c.ChatMessage.Hash, c.ChatMessage.Text, c.Engine.GetChannel())
	return true, err
}

type DispatchWhisperCommand struct{}

func (DispatchWhisperCommand) Handle(_ context.Context, c *Context) (bool, error) {
	if c.ChatMessage == nil {
		return false, nil
	}
	text := strings.TrimSpace(c.ChatMessage.Text)
	if !strings.HasPrefix(text, c.Engine.GetPrefix()) {
		return false, nil
	}
	f := strings.Fields(strings.TrimPrefix(text, c.Engine.GetPrefix()))
	if len(f) == 0 {
		return false, nil
	}
	cmd := common.BuildCommand(f[0], c.Engine, c.ChatMessage)
	if cmd != nil && c.Engine.IsUserAuthorized(c.ChatMessageUser(), cmd.GetRole()) {
		cmd.Execute()
	}
	return false, nil
}
func (c *Context) ChatMessageUser() *model.User {
	for u := range *c.Engine.GetActiveUsers() {
		if strings.EqualFold(u.Name, c.ChatMessage.Name) {
			return u
		}
	}
	return nil
}
func DefaultChain() *Chain {
	return NewChain(CaptureBanishedUser{}, IgnoreSelfWhisperInfo{}, RenameAfkUsers{}, ConvertWhisperToChatMessage{}, AuditWhisperCommand{}, DispatchWhisperCommand{})
}
