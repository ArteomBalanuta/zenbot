package contract

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSchemaValidationAndCanonicalJSON(t *testing.T) {
	s := SchemaObject(map[string]json.RawMessage{"a": SchemaString()}, []string{"a"}, false)
	if err := ValidateSchema(s, true); err != nil {
		t.Fatal(err)
	}
	if err := ValidateArguments(s, json.RawMessage(`{"a":"x"}`)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateArguments(s, json.RawMessage(`{"b":"x"}`)); err == nil {
		t.Fatal("expected required/unknown error")
	}
	a := json.RawMessage(`{"b":{"z":1,"a":2},"a":[2,1]}`)
	b := json.RawMessage(`{"a":[2,1],"b":{"a":2,"z":1}}`)
	if !bytes.Equal(CanonicalJSON(a), CanonicalJSON(b)) {
		t.Fatal("canonical mismatch")
	}
}
func TestEnvelope(t *testing.T) {
	e := SuccessEnvelope(json.RawMessage(`{"x":1}`))
	if string(e) != `{"status":"success","data":{"x":1}}` {
		t.Fatalf("%s", e)
	}
}

func TestVerifiedRoomDeliveryRequiresCommittedEffectAndPositiveReceipt(t *testing.T) {
	verified := ActionSuccessResult("call", "command", map[string]any{"deliveredCount": 1}, 1)
	if !verified.VerifiedRoomDelivery() || !verified.EffectsCommitted || verified.DeliveryCount != 1 {
		t.Fatalf("verified result=%#v", verified)
	}
	for _, result := range []Result{
		SuccessResult("call", "command", map[string]any{"deliveredCount": 1}),
		ActionSuccessResult("call", "command", map[string]any{"deliveredCount": 0}, 0),
		ErrorResult("call", "command", "COMMAND_REJECTED", "rejected"),
	} {
		if result.VerifiedRoomDelivery() {
			t.Fatalf("unverified result accepted: %#v", result)
		}
	}
}

func TestDescriptorOwnsValidatedModelResultLimit(t *testing.T) {
	newDescriptor := func(options ...DescriptorOption) (Descriptor, error) {
		return NewDescriptor(
			"bounded", "Bounded", "Return one bounded result.", "test",
			AccessUser, ReadOnly, ModelData,
			json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
			nil, nil, true, time.Second, json.RawMessage(`{"type":"object"}`),
			[]string{"test"}, nil, []string{"Do not use outside tests."}, options...,
		)
	}
	descriptor, err := newDescriptor()
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.MaxModelResultBytes() <= 0 {
		t.Fatalf("default model result limit=%d", descriptor.MaxModelResultBytes())
	}
	descriptor, err = newDescriptor(WithMaxModelResultBytes(4096))
	if err != nil || descriptor.MaxModelResultBytes() != 4096 {
		t.Fatalf("configured descriptor=%#v err=%v", descriptor, err)
	}
	if _, err := newDescriptor(WithMaxModelResultBytes(0)); err == nil {
		t.Fatal("zero model result limit was accepted")
	}
}

func TestValidateResultEnforcesRequiredObjectFields(t *testing.T) {
	s := SchemaObject(map[string]json.RawMessage{"answer": SchemaString()}, []string{"answer"}, false)
	if err := ValidateResult(s, json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected missing required result field")
	}
}

func TestRecursiveArgumentValidationRejectsNestedContractViolations(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"properties":{
			"filter":{
				"type":"object",
				"additionalProperties":false,
				"properties":{"nick":{"type":"string","minLength":1}},
				"required":["nick"]
			},
			"rooms":{
				"type":"array",
				"items":{"type":"string","minLength":1},
				"minItems":1,
				"maxItems":2
			}
		},
		"required":["filter","rooms"]
	}`)
	tests := []struct {
		name string
		args string
	}{
		{name: "nested required field", args: `{"filter":{},"rooms":["programming"]}`},
		{name: "nested additional property", args: `{"filter":{"nick":"jill","unknown":true},"rooms":["programming"]}`},
		{name: "array item type", args: `{"filter":{"nick":"jill"},"rooms":[7]}`},
		{name: "array min items", args: `{"filter":{"nick":"jill"},"rooms":[]}`},
		{name: "array max items", args: `{"filter":{"nick":"jill"},"rooms":["a","b","c"]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateArguments(schema, json.RawMessage(tc.args)); err == nil {
				t.Fatalf("ValidateArguments(%s) succeeded; want nested validation error", tc.args)
			}
		})
	}
	if err := ValidateArguments(schema, json.RawMessage(`{"filter":{"nick":"jill"},"rooms":["programming","lounge"]}`)); err != nil {
		t.Fatalf("valid nested arguments rejected: %v", err)
	}
}

func TestRecursiveResultValidationRejectsNestedContractViolations(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"properties":{
			"profile":{
				"type":"object",
				"additionalProperties":false,
				"properties":{"nick":{"type":"string","minLength":1}},
				"required":["nick"]
			},
			"rooms":{"type":"array","items":{"type":"string"},"minItems":1}
		},
		"required":["profile","rooms"]
	}`)
	for _, value := range []string{
		`{"profile":{},"rooms":["programming"]}`,
		`{"profile":{"nick":"jill","unknown":true},"rooms":["programming"]}`,
		`{"profile":{"nick":"jill"},"rooms":[3]}`,
		`{"profile":{"nick":"jill"},"rooms":[]}`,
	} {
		if err := ValidateResult(schema, json.RawMessage(value)); err == nil {
			t.Fatalf("ValidateResult(%s) succeeded; want nested validation error", value)
		}
	}
}

func TestRecursiveSchemaValidationRejectsInvalidNestedSchemas(t *testing.T) {
	for _, schema := range []string{
		`{"type":"object","properties":{"filter":{"type":"object","properties":{"nick":{"type":"unsupported"}}}}}`,
		`{"type":"object","properties":{"rooms":{"type":"array","items":{"type":"unsupported"}}}}`,
		`{"type":"object","properties":{"filter":{"type":"object","properties":{},"required":["missing"]}}}`,
		`{"type":"object","properties":{"name":{"type":"string","minLength":3,"maxLength":2}}}`,
		`{"type":"object","properties":{"rooms":{"type":"array","minItems":2,"maxItems":1}}}`,
	} {
		if err := ValidateSchema(json.RawMessage(schema), true); err == nil {
			t.Fatalf("ValidateSchema(%s) succeeded; want nested schema error", schema)
		}
	}
}

func TestSchemaRejectsUnsupportedKeywordsInsteadOfSilentlyDrifting(t *testing.T) {
	if err := ValidateSchema(json.RawMessage(`{"type":"object","properties":{},"pattern":".*"}`), true); err == nil {
		t.Fatal("unsupported pattern keyword was silently accepted")
	}
	if err := ValidateSchema(json.RawMessage(`{"type":"string","description":"supported annotation"}`), false); err != nil {
		t.Fatalf("description annotation rejected: %v", err)
	}
}

func TestConstValidationRequiresMatchingTypedLiteral(t *testing.T) {
	schema := json.RawMessage(`{"type":"string","const":"configure"}`)
	if err := ValidateSchema(schema, false); err != nil {
		t.Fatal(err)
	}
	if err := ValidateResult(schema, json.RawMessage(`"configure"`)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateResult(schema, json.RawMessage(`"enable"`)); err == nil {
		t.Fatal("non-constant value was accepted")
	}
	if err := ValidateSchema(json.RawMessage(`{"type":"integer","const":"one"}`), false); err == nil {
		t.Fatal("const with the wrong declared type was accepted")
	}
}

func TestOneOfValidationRequiresExactlyOneMatchingBranch(t *testing.T) {
	schema := json.RawMessage(`{"oneOf":[
		{"type":"object","additionalProperties":false,"properties":{"mode":{"type":"string","const":"exact"},"target":{"type":"string"}},"required":["mode","target"]},
		{"type":"object","additionalProperties":false,"properties":{"mode":{"type":"string","const":"multiple"},"targets":{"type":"array","items":{"type":"string"},"minItems":1}},"required":["mode","targets"]}
	]}`)
	if err := ValidateSchema(schema, false); err != nil {
		t.Fatal(err)
	}
	if err := ValidateResult(schema, json.RawMessage(`{"mode":"exact","target":"alice"}`)); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range []string{
		`{"mode":"contains","target":"alice"}`,
		`{"mode":"exact","target":"alice","targets":["alice"]}`,
	} {
		if err := ValidateResult(schema, json.RawMessage(arguments)); err == nil {
			t.Fatalf("arguments %s did not match exactly one branch", arguments)
		}
	}
	ambiguous := json.RawMessage(`{"oneOf":[{"type":"string","minLength":1},{"type":"string","maxLength":5}]}`)
	if err := ValidateResult(ambiguous, json.RawMessage(`"x"`)); err == nil {
		t.Fatal("value matching more than one branch was accepted")
	}
}

func TestOneOfSchemaRejectsMalformedBranchesAndRetainsNestedPath(t *testing.T) {
	for _, schema := range []string{
		`{"oneOf":[]}`,
		`{"oneOf":[{"type":"string"}]}`,
		`{"oneOf":[{"type":"string"},{"type":"unsupported"}]}`,
		`{"type":"object","oneOf":[{"type":"object"},{"type":"object"}]}`,
	} {
		if err := ValidateSchema(json.RawMessage(schema), false); err == nil {
			t.Fatalf("malformed oneOf schema accepted: %s", schema)
		}
	}
	nested := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"selection":{"oneOf":[{"type":"string","const":"one"},{"type":"string","const":"two"}]}},"required":["selection"]}`)
	err := ValidateArguments(nested, json.RawMessage(`{"selection":"three"}`))
	if err == nil || !strings.Contains(err.Error(), "arguments.selection") {
		t.Fatalf("nested conditional error lost its path: %v", err)
	}
}

func TestDescriptorRejectsBlankMetadataAndNegativeGuidance(t *testing.T) {
	params := SchemaObject(nil, nil, false)
	result := SchemaString()
	if _, err := NewDescriptor("tool", " ", "description", "test", AccessUser, ReadOnly, ModelData, params, nil, nil, true, 0, result, []string{"r"}, nil, []string{"never"}); err == nil {
		t.Fatal("expected blank label rejection")
	}
	if _, err := NewDescriptor("tool", "label", "description", "test", AccessUser, ReadOnly, ModelData, params, nil, nil, true, 0, result, []string{"r"}, nil, []string{" "}); err == nil {
		t.Fatal("expected blank negative guidance rejection")
	}
}

func TestDefinitionProjectsRoutingMetadataIntoProviderDescription(t *testing.T) {
	descriptor, err := NewDescriptor(
		"lookup", "User lookup", "Look up one user.", "users", AccessModerator,
		ReadOnly, ModelData, SchemaObject(nil, nil, false), []string{"MODERATION_COMMANDS"},
		[]string{"database_schema"}, true, time.Second, SchemaObject(nil, nil, true),
		[]string{"messages"}, nil, []string{"Do not use for room counts."},
	)
	if err != nil {
		t.Fatal(err)
	}
	description := NewDefinition(descriptor).Description
	for _, want := range []string{
		"Label: User lookup", "Category: users", "Access: MODERATOR", "Effect: READ_ONLY",
		"Result mode: MODEL_DATA", "Idempotent: true", "Required capabilities: MODERATION_COMMANDS",
		"Required successful tools: database_schema", "Reads: messages", "When not to use: Do not use for room counts.",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("provider description %q does not contain %q", description, want)
		}
	}
}

func FuzzValidateArgumentsNeverPanics(f *testing.F) {
	f.Add(`{"type":"object","additionalProperties":false,"properties":{"nick":{"type":"string"}}}`, `{"nick":"alice"}`)
	f.Add(`{"type":"object","properties":{"rows":{"type":"array","items":{"type":"integer"}}}}`, `{"rows":[1,2,3]}`)
	f.Add(`{`, `null`)
	f.Fuzz(func(t *testing.T, schema, arguments string) {
		_ = ValidateArguments(json.RawMessage(schema), json.RawMessage(arguments))
	})
}
