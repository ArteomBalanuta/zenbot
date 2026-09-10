package message

import (
	"context"
	"reflect"
	"strings"
	"unicode"
	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/profiling"
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
	profileHandlers := profiling.Active(ctx)
	for _, h := range c.handlers {
		var next bool
		var err error
		if profileHandlers {
			next, err = invokeHandler(ctx, h, state)
		} else {
			next, err = h.Handle(ctx, state)
		}
		if err != nil {
			return err
		}
		if !next {
			return nil
		}
	}
	return nil
}

func invokeHandler(ctx context.Context, handler Handler, state *Context) (next bool, err error) {
	done := profiling.Measure(ctx, handlerStageName(handler))
	defer done()
	return handler.Handle(ctx, state)
}

func handlerStageName(handler Handler) string {
	typeOf := reflect.TypeOf(handler)
	for typeOf != nil && typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	if typeOf == nil || typeOf.Name() == "" {
		return "handler.unknown"
	}
	var name strings.Builder
	for index, char := range typeOf.Name() {
		if unicode.IsUpper(char) && index > 0 {
			name.WriteByte('_')
		}
		name.WriteRune(unicode.ToLower(char))
	}
	return "handler." + name.String()
}
