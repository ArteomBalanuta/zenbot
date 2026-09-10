package message

import (
	"context"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type Context struct {
	Engine  common.Engine
	Message *model.ChatMessage
	Author  *model.User
}

type Handler interface {
	Handle(context.Context, *Context) (bool, error)
}
type HandlerFunc func(context.Context, *Context) (bool, error)

func (f HandlerFunc) Handle(ctx context.Context, c *Context) (bool, error) { return f(ctx, c) }

type Chain struct{ handlers []Handler }

func NewChain(h ...Handler) *Chain   { return &Chain{handlers: h} }
func (c *Chain) Handlers() []Handler { return append([]Handler(nil), c.handlers...) }
func (c *Chain) Process(ctx context.Context, message *model.ChatMessage, engine common.Engine) error {
	state := &Context{Engine: engine, Message: message}
	for _, h := range c.handlers {
		next, err := h.Handle(ctx, state)
		if err != nil {
			return err
		}
		if !next {
			return nil
		}
	}
	return nil
}
