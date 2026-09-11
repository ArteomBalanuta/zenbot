package info

import (
	"context"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type Context struct {
	Engine      common.Engine
	Message     *model.InfoMessage
	ChatMessage *model.ChatMessage
}
type Handler interface {
	Handle(context.Context, *Context) (bool, error)
}
type HandlerFunc func(context.Context, *Context) (bool, error)

func (f HandlerFunc) Handle(ctx context.Context, c *Context) (bool, error) { return f(ctx, c) }

type Chain struct{ handlers []Handler }

func NewChain(h ...Handler) *Chain   { return &Chain{handlers: h} }
func (c *Chain) Handlers() []Handler { return append([]Handler(nil), c.handlers...) }
func (c *Chain) Process(ctx context.Context, m *model.InfoMessage, e common.Engine) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s := &Context{Engine: e, Message: m}
	for _, h := range c.handlers {
		if err := ctx.Err(); err != nil {
			return err
		}
		next, err := h.Handle(ctx, s)
		if err != nil {
			return err
		}
		if !next {
			return nil
		}
	}
	return nil
}
