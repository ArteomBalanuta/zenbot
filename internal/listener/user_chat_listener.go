package listener

import (
	"context"
	"encoding/json"
	"log"
	"time"
	"zenbot/internal/common"
	"zenbot/internal/listener/message"
	"zenbot/internal/model"
	"zenbot/internal/profiling"
)

type UserChatListener struct {
	engine common.Engine
	chain  *message.Chain
}

type performanceProfilerProvider interface {
	PerformanceProfiler() *profiling.Profiler
}

func NewUserChatListener(e common.Engine) *UserChatListener {
	return NewUserChatListenerWithChain(e, nil)
}
func NewUserChatListenerWithChain(e common.Engine, chain *message.Chain) *UserChatListener {
	if chain == nil {
		chain = message.DefaultChain()
	}
	return &UserChatListener{engine: e, chain: chain}
}
func (u *UserChatListener) Notify(text string) {
	u.NotifyContext(context.Background(), text)
}

func (u *UserChatListener) NotifyContext(parent context.Context, text string) {
	if parent == nil {
		parent = context.Background()
	}
	var profiler *profiling.Profiler
	if provider, ok := u.engine.(performanceProfilerProvider); ok {
		candidate := provider.PerformanceProfiler()
		if candidate.Enabled() {
			profiler = candidate
		}
	}
	var parseStarted time.Time
	if profiler != nil {
		parseStarted = time.Now()
	}
	var m model.ChatMessage
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		log.Printf("malformed chat payload: %v", err)
		return
	}
	var parseDuration time.Duration
	if !parseStarted.IsZero() {
		parseDuration = time.Since(parseStarted)
	}
	m.IsWhisper = m.IsWhisper || m.Whisper || m.Type == "whisper"
	finish := func(error) {}
	if profiler != nil {
		parent, finish = profiler.StartCommand(parent, profiling.Command{
			Prefix:        u.engine.GetPrefix(),
			Text:          m.Text,
			Room:          u.engine.GetChannel(),
			Nick:          m.Name,
			Whisper:       m.IsWhisper,
			ParseDuration: parseDuration,
		})
	}
	messageCtx, cancel := context.WithCancel(parent)
	defer cancel()
	processErr := profiling.Run(messageCtx, func(profiled context.Context) error {
		return u.chain.Process(profiled, &m, u.engine)
	})
	finish(processErr)
	if processErr != nil {
		log.Printf("chat listener stopped: %v", processErr)
	}
}
