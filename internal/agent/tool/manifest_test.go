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
	return contract.NewDescriptor(
		t.name, t.name, "Read test data.", "test", contract.AccessUser,
		contract.ReadOnly, contract.ModelData,
		contract.SchemaObject(nil, nil, false), capabilities, nil, true, time.Second,
		contract.SchemaObject(nil, nil, true), []string{"test"}, nil,
		[]string{"Do not use for writes."},
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
