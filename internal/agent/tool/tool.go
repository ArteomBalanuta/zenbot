package tool

import (
	"context"
	"encoding/json"
	"fmt"
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

// InvocationAuthorizer optionally narrows a valid tool contract for a trusted
// invocation context. This is independent of descriptor capabilities so a
// denied context never requires a fabricated capability or invalid schema.
type InvocationAuthorizer interface {
	Authorized(api.Context) bool
}

// AuthorizedForInvocation reports whether a tool admits the trusted caller
// context in addition to its ordinary manifest and descriptor checks.
func AuthorizedForInvocation(t Tool, ctx api.Context) bool {
	authorizer, restricted := t.(InvocationAuthorizer)
	return !restricted || authorizer.Authorized(ctx)
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
	if !ok {
		return nil, false
	}
	if !AuthorizedForInvocation(t, ctx) {
		return nil, false
	}
	descriptor, err := t.Descriptor(ctx)
	if err != nil || descriptor.InternalFallback() {
		return nil, false
	}
	return t, ok
}
func (r *Registry) Lookup(n string) (Tool, bool) { t, ok := r.tools[n]; return t, ok }
func (r *Registry) Allowed(n string) bool        { return r.allow[n] }
func (r *Registry) Manifest(ctx api.Context) (contract.Manifest, error) {
	entries := make([]contract.ManifestEntry, 0, len(r.tools))
	intentOwners := make(map[string]string, len(r.tools))
	for name, registered := range r.tools {
		if !r.allow[name] {
			continue
		}
		if !AuthorizedForInvocation(registered, ctx) {
			continue
		}
		descriptor, err := registered.Descriptor(ctx)
		if err != nil {
			return contract.Manifest{}, fmt.Errorf("describe tool %s: %w", name, err)
		}
		if descriptor.InternalFallback() {
			continue
		}
		available := true
		for _, required := range descriptor.RequiredCapabilities() {
			if !ctx.HasCapability(api.Capability(required)) {
				available = false
				break
			}
		}
		if available {
			intent := descriptor.PrimaryIntent()
			if owner, duplicate := intentOwners[intent]; duplicate {
				return contract.Manifest{}, fmt.Errorf("primary intent %q is owned by both %q and %q", intent, owner, name)
			}
			intentOwners[intent] = name
			entries = append(entries, contract.NewManifestEntry(descriptor))
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return contract.Manifest{Version: contract.ManifestVersion, Tools: entries}, nil
}
func (r *Registry) Definitions(ctx api.Context) []contract.Definition {
	manifest, err := r.Manifest(ctx)
	if err != nil {
		return nil
	}
	return manifest.Definitions()
}
