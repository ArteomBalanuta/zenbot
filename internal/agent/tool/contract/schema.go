package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

func clone(v json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), v...) }
func SchemaString() json.RawMessage           { return json.RawMessage(`{"type":"string"}`) }
func SchemaObject(properties map[string]json.RawMessage, required []string, additional bool) json.RawMessage {
	m := map[string]any{"type": "object", "additionalProperties": additional}
	p := map[string]json.RawMessage{}
	for k, v := range properties {
		p[k] = clone(v)
	}
	m["properties"] = p
	if required != nil {
		m["required"] = required
	}
	b, _ := json.Marshal(m)
	return b
}
func ValidateSchema(raw json.RawMessage, parameters bool) error {
	var s map[string]json.RawMessage
	if json.Unmarshal(raw, &s) != nil {
		return fmt.Errorf("schema must be object")
	}
	var typ string
	if json.Unmarshal(s["type"], &typ) != nil || (parameters && typ != "object") || (!parameters && !supported(typ)) {
		return fmt.Errorf("invalid schema type")
	}
	if p, ok := s["properties"]; ok {
		var pm map[string]json.RawMessage
		if json.Unmarshal(p, &pm) != nil {
			return fmt.Errorf("properties must be object")
		}
		for n, v := range pm {
			var x map[string]json.RawMessage
			if json.Unmarshal(v, &x) != nil {
				return fmt.Errorf("property must be object: %s", n)
			}
			var t string
			if json.Unmarshal(x["type"], &t) != nil || !supported(t) {
				return fmt.Errorf("unsupported property type: %s", n)
			}
		}
	}
	if a, ok := s["additionalProperties"]; ok {
		var b bool
		if json.Unmarshal(a, &b) != nil {
			return fmt.Errorf("additionalProperties must be boolean")
		}
	}
	if r, ok := s["required"]; ok {
		var names []string
		if json.Unmarshal(r, &names) != nil {
			return fmt.Errorf("required must be array")
		}
		for _, n := range names {
			var p map[string]json.RawMessage
			json.Unmarshal(s["properties"], &p)
			if _, ok := p[n]; !ok {
				return fmt.Errorf("required name not declared: %s", n)
			}
		}
	}
	return nil
}
func supported(t string) bool {
	switch t {
	case "any", "string", "boolean", "number", "integer", "object", "array", "null":
		return true
	}
	return false
}
func ValidateArguments(schema, raw json.RawMessage) error {
	if err := ValidateSchema(schema, true); err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var a map[string]json.RawMessage
	if err := json.Unmarshal(raw, &a); err != nil {
		return fmt.Errorf("arguments must be object")
	}
	if a == nil {
		return fmt.Errorf("arguments must be object")
	}
	var s map[string]json.RawMessage
	json.Unmarshal(schema, &s)
	var p map[string]json.RawMessage
	json.Unmarshal(s["properties"], &p)
	var req []string
	json.Unmarshal(s["required"], &req)
	for _, n := range req {
		if _, ok := a[n]; !ok {
			return fmt.Errorf("missing required parameter: %s", n)
		}
	}
	var add bool
	if x, ok := s["additionalProperties"]; ok {
		json.Unmarshal(x, &add)
	}
	if !add {
		for n := range a {
			if _, ok := p[n]; !ok {
				return fmt.Errorf("unknown parameter: %s", n)
			}
		}
	}
	for n, v := range a {
		if ps, ok := p[n]; ok {
			if err := validateValue(n, ps, v); err != nil {
				return err
			}
		}
	}
	return nil
}
func validateValue(n string, raw, v json.RawMessage) error {
	var s map[string]json.RawMessage
	json.Unmarshal(raw, &s)
	var t string
	json.Unmarshal(s["type"], &t)
	var x any
	if json.Unmarshal(v, &x) != nil {
		return fmt.Errorf("invalid type for parameter: %s", n)
	}
	valid := true
	switch t {
	case "string":
		_, valid = x.(string)
	case "boolean":
		_, valid = x.(bool)
	case "number":
		_, valid = x.(float64)
	case "integer":
		f, ok := x.(float64)
		valid = ok && f == math.Trunc(f)
	case "object":
		_, valid = x.(map[string]any)
	case "array":
		_, valid = x.([]any)
	case "null":
		valid = x == nil
	}
	if !valid {
		return fmt.Errorf("invalid type for parameter: %s", n)
	}
	var enum []json.RawMessage
	if json.Unmarshal(s["enum"], &enum) == nil && len(enum) > 0 {
		found := false
		for _, e := range enum {
			if bytes.Equal(CanonicalJSON(e), CanonicalJSON(v)) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("invalid value for parameter: %s", n)
		}
	}
	if str, ok := x.(string); ok {
		var min, max int
		if json.Unmarshal(s["minLength"], &min) == nil && utf8.RuneCountInString(str) < min {
			return fmt.Errorf("parameter is shorter than allowed: %s", n)
		}
		if json.Unmarshal(s["maxLength"], &max) == nil && utf8.RuneCountInString(str) > max {
			return fmt.Errorf("parameter is longer than allowed: %s", n)
		}
	}
	if f, ok := x.(float64); ok {
		var min, max float64
		if json.Unmarshal(s["minimum"], &min) == nil && f < min {
			return fmt.Errorf("parameter is below minimum: %s", n)
		}
		if json.Unmarshal(s["maximum"], &max) == nil && f > max {
			return fmt.Errorf("parameter is above maximum: %s", n)
		}
	}
	return nil
}
func ValidateResult(schema, value json.RawMessage) error {
	if err := ValidateSchema(schema, false); err != nil {
		return err
	}
	var s map[string]json.RawMessage
	json.Unmarshal(schema, &s)
	var t string
	json.Unmarshal(s["type"], &t)
	if err := validateValue("result", schema, value); err != nil {
		return fmt.Errorf("tool result does not match declared %s schema", t)
	}
	if t == "object" {
		var sreq []string
		json.Unmarshal(s["required"], &sreq)
		var object map[string]json.RawMessage
		json.Unmarshal(value, &object)
		for _, name := range sreq {
			if _, ok := object[name]; !ok {
				return fmt.Errorf("tool result is missing required field: %s", name)
			}
		}
	}
	return nil
}
func CanonicalJSON(raw json.RawMessage) json.RawMessage {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return clone(raw)
	}
	return canonical(v)
}
func canonical(v any) json.RawMessage {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			b.Write(kb)
			b.WriteByte(':')
			b.Write(canonical(x[k]))
		}
		b.WriteByte('}')
		return []byte(b.String())
	case []any:
		var b strings.Builder
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			b.Write(canonical(e))
		}
		b.WriteByte(']')
		return []byte(b.String())
	default:
		b, _ := json.Marshal(v)
		return b
	}
}
func SuccessEnvelope(data json.RawMessage) json.RawMessage {
	if !json.Valid(data) {
		data, _ = json.Marshal(string(data))
	}
	return append([]byte(`{"status":"success","data":`), append(data, '}')...)
}
func ErrorEnvelope(code, msg string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"status": "error", "data": nil, "error": map[string]string{"code": code, "message": msg}})
	return b
}
