package main

import (
	"fmt"
	"sync"

	"zenbot/internal/core"
)

// masterBinding is main's synchronized current-master reference.
type masterBinding struct {
	mu sync.RWMutex
	e  *core.EngineImpl
}

func newMasterBinding(engine *core.EngineImpl) *masterBinding { return &masterBinding{e: engine} }

func (b *masterBinding) Current() *core.EngineImpl {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.e
}

func (b *masterBinding) Rebind(next *core.EngineImpl) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.e = next
	b.mu.Unlock()
}

func masterReplySender(binding *masterBinding) func(string, string, bool) (string, error) {
	return func(author, reply string, whisper bool) (string, error) {
		current := binding.Current()
		if current == nil {
			return "", fmt.Errorf("current master is not constructed")
		}
		return current.SendChatMessage(author, reply, whisper)
	}
}
