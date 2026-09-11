package contract

import (
	"encoding/json"
	"testing"
)

func TestStrictParametersRequireCompleteClosedObjectSchemas(t *testing.T) {
	for _, test := range []struct {
		name   string
		schema string
		strict bool
	}{
		{"empty arguments", `{"type":"object","properties":{},"additionalProperties":false}`, true},
		{"required nickname", `{"type":"object","properties":{"nick":{"type":"string"}},"required":["nick"],"additionalProperties":false}`, true},
		{"optional limit", `{"type":"object","properties":{"limit":{"type":"integer"}},"additionalProperties":false}`, false},
		{"open object", `{"type":"object","properties":{}}`, false},
		{"root union", `{"oneOf":[{"type":"object","properties":{"a":{"type":"string"}},"required":["a"],"additionalProperties":false},{"type":"object","properties":{"b":{"type":"string"}},"required":["b"],"additionalProperties":false}]}`, false},
		{"nested optional field", `{"type":"object","properties":{"data":{"type":"object","properties":{"nick":{"type":"string"}},"additionalProperties":false}},"required":["data"],"additionalProperties":false}`, false},
		{"array of required objects", `{"type":"object","properties":{"users":{"type":"array","items":{"type":"object","properties":{"nick":{"type":"string"}},"required":["nick"],"additionalProperties":false}}},"required":["users"],"additionalProperties":false}`, true},
		{"array of open objects", `{"type":"object","properties":{"users":{"type":"array","items":{"type":"object","properties":{}}}},"required":["users"],"additionalProperties":false}`, false},
		{"local any type", `{"type":"object","properties":{"data":{"type":"any"}},"required":["data"],"additionalProperties":false}`, false},
		{"invalid JSON", `{`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := SupportsStrictParameters(json.RawMessage(test.schema)); got != test.strict {
				t.Fatalf("strict support = %t, want %t for %s", got, test.strict, test.schema)
			}
		})
	}
}

func TestParameterSchemaRequiresExplicitRootObject(t *testing.T) {
	union := json.RawMessage(`{"oneOf":[{"type":"object","properties":{"x":{"type":"string"}},"required":["x"]},{"type":"object","properties":{"y":{"type":"string"}},"required":["y"]}]}`)
	if err := ValidateSchema(union, true); err == nil {
		t.Fatal("root union cannot be advertised as a function parameter object")
	}
	if err := ValidateSchema(union, false); err != nil {
		t.Fatalf("result union must remain supported: %v", err)
	}
}

func TestArgumentAndResultValidationRejectTrailingJSON(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	for _, raw := range []string{`{} {}`, `{} trailing`} {
		if err := ValidateArguments(schema, json.RawMessage(raw)); err == nil {
			t.Errorf("accepted malformed arguments: %s", raw)
		}
		if err := ValidateResult(schema, json.RawMessage(raw)); err == nil {
			t.Errorf("accepted malformed result: %s", raw)
		}
	}
}
