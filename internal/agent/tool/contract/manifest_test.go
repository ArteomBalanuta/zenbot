package contract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRoutingMetadataIsValidatedAndDefensivelyCopied(t *testing.T) {
	aliases := []string{"whois"}
	targets := []string{"user"}
	useWhen := []string{"The caller asks for a user's current identity."}
	examples := []Example{{
		Prompt:    "Who is @jill?",
		Arguments: json.RawMessage(`{"nick":"jill"}`),
	}}
	descriptor, err := NewDescriptor(
		"saturn_info", "User info", "Look up one user's current identity.", "users",
		AccessUser, ReadOnly, ModelData,
		json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"nick":{"type":"string","minLength":1}},"required":["nick"]}`),
		nil, nil, true, time.Second, json.RawMessage(`{"type":"object"}`),
		[]string{"room_users"}, nil, []string{"Do not use for message history."},
		WithRouting(RoutingMetadata{
			Aliases:  aliases,
			Targets:  targets,
			UseWhen:  useWhen,
			Examples: examples,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	aliases[0] = "mutated"
	targets[0] = "mutated"
	useWhen[0] = "mutated"
	examples[0].Arguments[0] = '['
	routing := descriptor.Routing()
	if !reflect.DeepEqual(routing.Aliases, []string{"whois"}) ||
		!reflect.DeepEqual(routing.Targets, []string{"user"}) ||
		!reflect.DeepEqual(routing.UseWhen, []string{"The caller asks for a user's current identity."}) ||
		string(routing.Examples[0].Arguments) != `{"nick":"jill"}` {
		t.Fatalf("descriptor routing metadata was mutated: %#v", routing)
	}
	routing.Aliases[0] = "changed again"
	routing.Examples[0].Arguments[0] = '['
	if got := descriptor.Routing(); got.Aliases[0] != "whois" || string(got.Examples[0].Arguments) != `{"nick":"jill"}` {
		t.Fatalf("routing accessor leaked mutable state: %#v", got)
	}
}

func TestRoutingMetadataRejectsMalformedExamplesAndBlankGuidance(t *testing.T) {
	newDescriptor := func(routing RoutingMetadata) error {
		_, err := NewDescriptor(
			"lookup", "Lookup", "Look up one value.", "test", AccessUser, ReadOnly, ModelData,
			json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","minLength":1}},"required":["value"]}`),
			nil, nil, true, time.Second, json.RawMessage(`{"type":"object"}`),
			[]string{"values"}, nil, []string{"Do not use for writes."}, WithRouting(routing),
		)
		return err
	}

	for name, routing := range map[string]RoutingMetadata{
		"blank target":       {Targets: []string{" "}},
		"blank use guidance": {UseWhen: []string{" "}},
		"blank prompt":       {Examples: []Example{{Prompt: " ", Arguments: json.RawMessage(`{"value":"x"}`)}}},
		"malformed JSON":     {Examples: []Example{{Prompt: "look up x", Arguments: json.RawMessage(`{`)}}},
		"schema mismatch":    {Examples: []Example{{Prompt: "look up x", Arguments: json.RawMessage(`{"unknown":"x"}`)}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := newDescriptor(routing); err == nil {
				t.Fatal("expected routing validation error")
			}
		})
	}
}

func TestManifestJSONRoundTripPreservesContracts(t *testing.T) {
	descriptor, err := NewDescriptor(
		"lookup", "Lookup", "Look up one value.", "test", AccessModerator, ReadOnly, ModelData,
		SchemaObject(map[string]json.RawMessage{"value": SchemaString()}, []string{"value"}, false),
		[]string{"MODERATION_COMMANDS"}, []string{"schema"}, true, 250*time.Millisecond,
		SchemaObject(map[string]json.RawMessage{"value": SchemaString()}, []string{"value"}, false),
		[]string{"values"}, nil, []string{"Do not use for writes."},
		WithRouting(RoutingMetadata{
			Aliases:  []string{"find"},
			Targets:  []string{"value"},
			UseWhen:  []string{"A fresh value is needed."},
			Examples: []Example{{Prompt: "Find x", Arguments: json.RawMessage(`{"value":"x"}`)}},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := Manifest{Version: ManifestVersion, Tools: []ManifestEntry{NewManifestEntry(descriptor)}}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got Manifest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("manifest round trip mismatch\nwant: %#v\n got: %#v", want, got)
	}
	definition := got.Tools[0].Definition()
	for _, text := range []string{"Aliases: find", "Targets: value", "Use when: A fresh value is needed.", `Example: Find x => {"value":"x"}`} {
		if !strings.Contains(definition.Description, text) {
			t.Fatalf("provider projection %q does not contain %q", definition.Description, text)
		}
	}
}

func TestProviderDefinitionPreservesPurposePrerequisitesAndDelivery(t *testing.T) {
	entry := ManifestEntry{
		Name: "saturn_ping", Description: "Measure TCP latency to hack.chat:80.",
		Routing:    RoutingMetadata{UseWhen: []string{"Use when the caller asks to ping the bot."}, Examples: []Example{{Prompt: "Ping the bot", Arguments: json.RawMessage(`{}`)}}},
		WhenNotUse: []string{"The target is fixed; no host argument is accepted."},
		Effect:     Action, ResultMode: RoomDelivery,
		RequiredSuccessfulTools: []string{"database_schema"},
	}
	definition := entry.ProviderDefinition()
	for _, required := range []string{entry.Description, entry.Routing.UseWhen[0], entry.WhenNotUse[0], "database_schema", "delivers", "room", "Example: Ping the bot", "{}"} {
		if !strings.Contains(definition.Description, required) {
			t.Fatalf("provider omitted useful contract %q: %s", required, definition.Description)
		}
	}
	for _, abbreviation := range []string{"I=", "L=", "P=", "Args="} {
		if strings.Contains(definition.Description, abbreviation) {
			t.Fatalf("provider requires interpreting metadata notation %q: %s", abbreviation, definition.Description)
		}
	}
}

func TestProviderDefinitionExplainsSilentActionReceipt(t *testing.T) {
	entry := ManifestEntry{Name: "saturn_kick", Description: "Kick one active nickname.", Effect: Action, ResultMode: ModelData}
	definition := entry.ProviderDefinition()
	if !strings.Contains(definition.Description, "documented action receipt") || !strings.Contains(definition.Description, "room message") {
		t.Fatalf("silent action contract is not explained: %s", definition.Description)
	}
}
