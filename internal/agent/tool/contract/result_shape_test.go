package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestProviderDefinitionProjectsSortedOptionalNestedResultShapeWithoutChangingParameters(t *testing.T) {
	parameters := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"query":{"type":"string"}},"required":["query"]}`)
	entry := ManifestEntry{
		Name:        "room_snapshot",
		Description: "Read one room snapshot.",
		Parameters:  parameters,
		ResultSchema: json.RawMessage(`{
			"type":"object",
			"description":"This verbose schema description must not be copied into provider guidance.",
			"additionalProperties":false,
			"properties":{
				"users":{"type":"array","maxItems":500,"items":{"type":"object","additionalProperties":false,"properties":{"trip":{"type":"string"},"nick":{"type":"string"}},"required":["nick","trip"]}},
				"room":{"type":"string"},
				"note":{"type":"string"},
				"count":{"type":"integer","minimum":0}
			},
			"required":["users","room","count"]
		}`),
	}

	definition := entry.ProviderDefinition()
	wantShape := "Result shape (not arguments): {count:integer, note?:string, room:string, users:array<{nick:string, trip:string}>}."
	if !strings.Contains(definition.Description, wantShape) {
		t.Fatalf("provider result guidance = %q, want it to contain %q", definition.Description, wantShape)
	}
	if strings.Contains(definition.Description, "verbose schema description") || strings.Contains(definition.Description, "maxItems") {
		t.Fatalf("provider result guidance copied verbose validation metadata: %s", definition.Description)
	}
	if !bytes.Equal(definition.Parameters, parameters) {
		t.Fatalf("result fields leaked into arguments\nwant: %s\n got: %s", parameters, definition.Parameters)
	}
}

func TestProviderDefinitionRepresentsEverySupportedResultSchemaType(t *testing.T) {
	entry := ManifestEntry{
		Description: "Read typed data.",
		ResultSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"anything":{"type":"any"},
				"flag":{"type":"boolean"},
				"fraction":{"type":"number"},
				"items":{"type":"array","items":{"type":"null"}},
				"nested":{"type":"object","properties":{}},
				"selection":{"oneOf":[{"type":"string"},{"type":"integer"}]}
			},
			"required":["anything","flag","fraction","items","nested","selection"]
		}`),
	}

	definition := entry.ProviderDefinition()
	want := "{anything:any, flag:boolean, fraction:number, items:array<null>, nested:object, selection:oneOf<integer|string>}"
	if !strings.Contains(definition.Description, want) {
		t.Fatalf("provider type projection = %q, want it to contain %q", definition.Description, want)
	}
}

func TestProviderDefinitionBoundsLargeUTF8ResultShapeWithExplicitOmission(t *testing.T) {
	properties := make(map[string]json.RawMessage, 80)
	required := make([]string, 0, 80)
	for index := 0; index < 80; index++ {
		name := fmt.Sprintf("donnée_%03d_界", index)
		properties[name] = json.RawMessage(`{"type":"string","description":"unused verbose text"}`)
		required = append(required, name)
	}
	entry := ManifestEntry{Description: "Read large data.", ResultSchema: SchemaObject(properties, required, false)}

	definition := entry.ProviderDefinition()
	start := strings.Index(definition.Description, "Result shape (not arguments):")
	if start < 0 {
		t.Fatalf("provider omitted result shape: %s", definition.Description)
	}
	hint := definition.Description[start:]
	if len(hint) > 512 {
		t.Fatalf("result shape hint is %d bytes, want at most 512: %q", len(hint), hint)
	}
	if !utf8.ValidString(hint) {
		t.Fatalf("bounded result shape is not valid UTF-8: %q", hint)
	}
	if !strings.Contains(hint, "[... omitted]") {
		t.Fatalf("bounded result shape does not mark omitted content: %q", hint)
	}
}

func TestProviderDefinitionMarksNestingOmittedAndSkipsAbsentSchema(t *testing.T) {
	deep := json.RawMessage(`{"type":"object","properties":{"a":{"type":"object","properties":{"b":{"type":"object","properties":{"c":{"type":"object","properties":{"d":{"type":"string"}},"required":["d"]}},"required":["c"]}},"required":["b"]}},"required":["a"]}`)
	definition := (ManifestEntry{Description: "Read deep data.", ResultSchema: deep}).ProviderDefinition()
	if !strings.Contains(definition.Description, "[... omitted]") {
		t.Fatalf("shallow projection does not mark omitted nesting: %s", definition.Description)
	}

	withoutSchema := (ManifestEntry{Description: "No declared result schema."}).ProviderDefinition()
	if strings.Contains(withoutSchema.Description, "Result shape") {
		t.Fatalf("provider invented result guidance for an absent schema: %s", withoutSchema.Description)
	}
}
