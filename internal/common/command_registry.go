package common

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"zenbot/internal/model"
)

// SaturnCommand is the context-aware command contract used by migrated handlers.
type SaturnCommand interface {
	Execute(context.Context) (model.Status, error)
	Role() model.Role
	Aliases() []string
	NewInstance(Engine, *model.ChatMessage) SaturnCommand
}
type CommandDefinition struct {
	Canonical string
	Aliases   []string
	Role      model.Role
	Whisper   bool
	New       func(Engine, *model.ChatMessage) SaturnCommand
}
type SaturnCommandRegistry struct {
	defs    []CommandDefinition
	byAlias map[string]CommandDefinition
}

func NewSaturnCommandRegistry() *SaturnCommandRegistry {
	return &SaturnCommandRegistry{byAlias: map[string]CommandDefinition{}}
}
func norm(s string) string {
	r := []rune(strings.ToLower(strings.TrimSpace(s)))
	sort.Slice(r, func(i, j int) bool { return r[i] < r[j] })
	return string(r)
}
func (r *SaturnCommandRegistry) Register(d CommandDefinition) error {
	if d.Canonical == "" || d.New == nil {
		return fmt.Errorf("invalid command definition %q", d.Canonical)
	}
	seen := map[string]bool{}
	for _, a := range append([]string{d.Canonical}, d.Aliases...) {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			return fmt.Errorf("duplicate alias %q", a)
		}
		// Catalogs may list the canonical command among its aliases.
		// Store it once while still rejecting repeated non-canonical aliases.
		if seen[a] {
			if a == strings.ToLower(strings.TrimSpace(d.Canonical)) {
				continue
			}
			return fmt.Errorf("duplicate alias %q", a)
		}
		seen[a] = true
		if _, ok := r.byAlias[a]; ok {
			return fmt.Errorf("duplicate alias %q", a)
		}
		for _, old := range r.defs {
			for _, oa := range append([]string{old.Canonical}, old.Aliases...) {
				if norm(a) == norm(oa) {
					return fmt.Errorf("anagram alias collision %q/%q", a, oa)
				}
			}
		}
	}
	r.defs = append(r.defs, d)
	for a := range seen {
		r.byAlias[a] = d
	}
	return nil
}
func (r *SaturnCommandRegistry) Lookup(a string) (CommandDefinition, bool) {
	d, ok := r.byAlias[strings.ToLower(strings.TrimSpace(a))]
	return d, ok
}
func (r *SaturnCommandRegistry) Definitions() []CommandDefinition {
	out := append([]CommandDefinition(nil), r.defs...)
	sort.Slice(out, func(i, j int) bool { return out[i].Canonical < out[j].Canonical })
	return out
}
func (r *SaturnCommandRegistry) Validate() error {
	for _, d := range r.defs {
		if len(d.Aliases) == 0 {
			return fmt.Errorf("command %q has no aliases", d.Canonical)
		}
	}
	return nil
}
