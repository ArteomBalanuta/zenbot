package tool

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/tool/contract"
)

type Tool interface {
	Name() string
	Descriptor(api.Context) (contract.Descriptor, error)
	Execute(context.Context, api.Context, json.RawMessage) (contract.Result, error)
}

// ArgumentString returns only nonblank JSON string primitives.
func ArgumentString(args json.RawMessage, name string) string {
	var m map[string]json.RawMessage
	if json.Unmarshal(args, &m) != nil {
		return ""
	}
	var s string
	if json.Unmarshal(m[name], &s) != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

type Registry struct {
	tools map[string]Tool
	allow map[string]bool
}

func NewRegistry(tools []Tool, allowed []string) *Registry {
	r := &Registry{tools: map[string]Tool{}, allow: map[string]bool{}}
	for _, t := range tools {
		r.tools[t.Name()] = t
	}
	for _, n := range allowed {
		r.allow[n] = true
	}
	return r
}
func (r *Registry) Find(ctx api.Context, n string) (Tool, bool) {
	if !r.allow[n] {
		return nil, false
	}
	t, ok := r.tools[n]
	return t, ok
}
func (r *Registry) Lookup(n string) (Tool, bool) { t, ok := r.tools[n]; return t, ok }
func (r *Registry) Allowed(n string) bool        { return r.allow[n] }
func (r *Registry) Definitions(ctx api.Context) []contract.Definition {
	out := []contract.Definition{}
	for n, t := range r.tools {
		if !r.allow[n] {
			continue
		}
		if d, e := t.Descriptor(ctx); e == nil {
			out = append(out, contract.NewDefinition(d))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
