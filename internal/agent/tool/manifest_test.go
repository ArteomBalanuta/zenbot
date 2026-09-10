package tool

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/tool/contract"
)

type manifestTestTool struct {
	name          string
	capability    api.Capability
	descriptorErr error
	primaryIntent string
	internalOnly  bool
}

func (t manifestTestTool) Name() string { return t.name }

func (t manifestTestTool) Descriptor(api.Context) (contract.Descriptor, error) {
	if t.descriptorErr != nil {
		return contract.Descriptor{}, t.descriptorErr
	}
	capabilities := []string(nil)
	if t.capability != "" {
		capabilities = []string{string(t.capability)}
	}
	options := []contract.DescriptorOption(nil)
	if t.primaryIntent != "" {
		options = append(options, contract.WithPrimaryIntent(t.primaryIntent))
	}
	if t.internalOnly {
		options = append(options, contract.WithInternalFallback())
	}
	return contract.NewDescriptor(
		t.name, t.name, "Read test data.", "test", contract.AccessUser,
		contract.ReadOnly, contract.ModelData,
		contract.SchemaObject(nil, nil, false), capabilities, nil, true, time.Second,
		contract.SchemaObject(nil, nil, true), []string{"test"}, nil,
		[]string{"Do not use for writes."}, options...,
	)
}

func (t manifestTestTool) Execute(context.Context, api.Context, json.RawMessage) (contract.Result, error) {
	return contract.SuccessResult("call", t.name, map[string]any{}), nil
}

func TestManifestIsDeterministicAndCapabilityFiltered(t *testing.T) {
	registry := NewRegistry(
		[]Tool{
			manifestTestTool{name: "zeta"},
			manifestTestTool{name: "admin_only", capability: api.AdminCommands},
			manifestTestTool{name: "alpha"},
		},
		[]string{"zeta", "admin_only", "alpha"},
	)
	public, err := api.NewContext("programming", "jill", "", "hash", false, []string{"jill"})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := registry.Manifest(public)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != contract.ManifestVersion {
		t.Fatalf("manifest version = %q", manifest.Version)
	}
	got := make([]string, 0, len(manifest.Tools))
	for _, entry := range manifest.Tools {
		got = append(got, entry.Name)
	}
	if want := []string{"alpha", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("manifest names = %v, want %v", got, want)
	}
}

func TestManifestReturnsDescriptorErrors(t *testing.T) {
	want := errors.New("invalid descriptor")
	registry := NewRegistry([]Tool{manifestTestTool{name: "broken", descriptorErr: want}}, []string{"broken"})
	caller, err := api.NewContext("programming", "jill", "", "hash", false, []string{"jill"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Manifest(caller); !errors.Is(err, want) {
		t.Fatalf("Manifest() error = %v, want %v", err, want)
	}
}

func TestManifestRejectsDuplicatePrimaryIntentOwners(t *testing.T) {
	registry := NewRegistry([]Tool{
		manifestTestTool{name: "managed_presence", primaryIntent: "room_presence"},
		manifestTestTool{name: "remote_presence", primaryIntent: "room_presence"},
	}, []string{"managed_presence", "remote_presence"})
	caller, err := api.NewContext("programming", "jill", "", "hash", false, []string{"jill"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := registry.Manifest(caller); err == nil {
		t.Fatal("duplicate primary intent was exposed to the model")
	}
	if definitions := registry.Definitions(caller); len(definitions) != 0 {
		t.Fatalf("colliding definitions escaped through compatibility API: %#v", definitions)
	}
}

func TestManifestOmitsInternalFallbackProviders(t *testing.T) {
	registry := NewRegistry([]Tool{
		manifestTestTool{name: "authoritative", primaryIntent: "room_presence"},
		manifestTestTool{name: "legacy_fallback", primaryIntent: "room_presence", internalOnly: true},
	}, []string{"authoritative", "legacy_fallback"})
	caller, err := api.NewContext("programming", "jill", "", "hash", false, []string{"jill"})
	if err != nil {
		t.Fatal(err)
	}

	manifest, err := registry.Manifest(caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Tools) != 1 || manifest.Tools[0].Name != "authoritative" || manifest.Tools[0].PrimaryIntent != "room_presence" {
		t.Fatalf("manifest tools = %#v", manifest.Tools)
	}
	if _, found := registry.Find(caller, "legacy_fallback"); found {
		t.Fatal("internal fallback remained callable through the model execution registry")
	}
}
