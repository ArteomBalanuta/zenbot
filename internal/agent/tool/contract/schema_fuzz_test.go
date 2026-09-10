package contract

import (
	"encoding/json"
	"testing"
)

func FuzzConditionalSchemaValidationNeverPanics(f *testing.F) {
	f.Add(`{"oneOf":[{"type":"string"},{"type":"string"}]}`, `"ambiguous"`)
	f.Add(`{"oneOf":[{"type":"object","properties":{"mode":{"type":"string","const":"a"}}},{"type":"object","properties":{"mode":{"type":"string","const":"b"}}}]}`, `{"mode":"c"}`)
	f.Add(`{"oneOf":[`, `{`)
	f.Add(`{"type":"integer","const":"wrong"}`, `1`)
	f.Fuzz(func(t *testing.T, schema, value string) {
		rawSchema, rawValue := json.RawMessage(schema), json.RawMessage(value)
		_ = ValidateSchema(rawSchema, false)
		_ = ValidateResult(rawSchema, rawValue)
	})
}
