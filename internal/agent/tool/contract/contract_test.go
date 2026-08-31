package contract

import (
	"bytes"
	"encoding/json"
	"testing"
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

func TestValidateResultEnforcesRequiredObjectFields(t *testing.T) {
	s := SchemaObject(map[string]json.RawMessage{"answer": SchemaString()}, []string{"answer"}, false)
	if err := ValidateResult(s, json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected missing required result field")
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
