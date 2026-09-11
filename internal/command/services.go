package command

import (
	"context"
	"fmt"
	"strings"
	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

type serviceEngine interface{ ServiceBundle() *service.Bundle }

func bundle(e common.Engine) *service.Bundle {
	if p, ok := e.(serviceEngine); ok {
		return p.ServiceBundle()
	}
	return nil
}

type pingCommand struct{ commandBase }

func (c *pingCommand) Execute(ctx context.Context) (model.Status, error) {
	if e := ctx.Err(); e != nil {
		return model.FAILED, e
	}
	b := bundle(c.engine)
	if b == nil || b.Ping == nil {
		return model.FAILED, fmt.Errorf("ping service unavailable")
	}
	d, e := b.Ping.Ping(ctx)
	if e != nil {
		return model.FAILED, e
	}
	if _, err := c.engine.SendChatMessage(c.message.Name, fmt.Sprintf("response time: %d milliseconds", d.Milliseconds()), c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type weatherCommand struct{ commandBase }

func (c *weatherCommand) Execute(ctx context.Context) (model.Status, error) {
	if e := ctx.Err(); e != nil {
		return model.FAILED, e
	}
	a := args(c.message)
	if len(a) == 0 {
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.Weather == nil {
		return model.FAILED, fmt.Errorf("weather service unavailable")
	}
	v, e := b.Weather.Get(ctx, strings.Join(a, " "))
	if e != nil {
		return model.FAILED, e
	}
	if _, err := c.engine.SendChatMessage(c.message.Name, v, c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type timeCommand struct{ commandBase }

func (c *timeCommand) Execute(ctx context.Context) (model.Status, error) {
	if e := ctx.Err(); e != nil {
		return model.FAILED, e
	}
	a := args(c.message)
	if len(a) == 0 {
		prefix := "!"
		if bundle(c.engine) != nil {
			prefix = c.engine.GetPrefix()
		}
		if _, err := c.engine.SendChatMessage(c.message.Name, fmt.Sprintf("Example: %stime Tokyo", prefix), c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.Time == nil {
		return model.FAILED, fmt.Errorf("time service unavailable")
	}
	v, e := b.Time.Get(ctx, strings.Join(a, " "))
	if e != nil {
		return model.FAILED, e
	}
	if _, err := c.engine.SendChatMessage(c.message.Name, v, c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
