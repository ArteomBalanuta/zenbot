package common

import (
	"fmt"
	"strings"
	"sync"

	"zenbot/internal/model"
)

// RuntimeCommandRegistry owns aliases registered by a single command instance.
// Its zero value is ready for registration; snapshots never expose its maps.
type RuntimeCommandRegistry struct {
	mu       sync.RWMutex
	exact    map[string]CommandMetadata
	anagrams map[string]CommandMetadata
}

func (r *RuntimeCommandRegistry) Register(c Command, engine Engine) error {
	if c == nil {
		return fmt.Errorf("nil command")
	}
	aliases := append([]string(nil), c.GetAliases()...)
	if len(aliases) == 0 {
		return fmt.Errorf("command has no aliases")
	}
	seen := make(map[string]bool, len(aliases))
	for _, alias := range aliases {
		alias = strings.ToLower(strings.TrimSpace(alias))
		if alias == "" {
			return fmt.Errorf("command has a blank alias")
		}
		seen[alias] = true
	}
	constructor := func(msg *model.ChatMessage) Command { return c.NewInstance(engine, msg) }
	r.mu.Lock()
	defer r.mu.Unlock()
	for alias := range seen {
		if _, exists := r.exact[alias]; exists {
			return fmt.Errorf("duplicate alias %q", alias)
		}
		if _, exists := r.anagrams[commandAnagramKey(alias)]; exists {
			return fmt.Errorf("anagram alias collision %q", alias)
		}
	}
	if r.exact == nil {
		r.exact = make(map[string]CommandMetadata)
		r.anagrams = make(map[string]CommandMetadata)
	}
	for _, alias := range aliases {
		metadata := CommandMetadata{Alias: alias, Command: constructor}
		r.exact[strings.ToLower(strings.TrimSpace(alias))] = metadata
		r.anagrams[commandAnagramKey(alias)] = metadata
	}
	return nil
}

func (r *RuntimeCommandRegistry) Lookup(alias string) (CommandMetadata, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	metadata, ok := r.exact[strings.ToLower(strings.TrimSpace(alias))]
	if !ok {
		metadata, ok = r.anagrams[commandAnagramKey(alias)]
	}
	return metadata, ok
}

func (r *RuntimeCommandRegistry) Snapshot() map[string]CommandMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]CommandMetadata, len(r.exact))
	for alias, metadata := range r.exact {
		out[alias] = metadata
	}
	return out
}
