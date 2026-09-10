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
	typ, err := validateSchemaNode(raw, "$schema")
	if err != nil {
		return err
	}
	if parameters && typ != "object" {
		return fmt.Errorf("parameter schema must be object")
	}
	return nil
}

func validateSchemaNode(raw json.RawMessage, path string) (string, error) {
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		return "", fmt.Errorf("%s must be an object", path)
	}
	for keyword := range schema {
		if !supportedSchemaKeyword(keyword) {
			return "", fmt.Errorf("%s contains unsupported keyword %s", path, keyword)
		}
	}
	if rawOneOf, conditional := schema["oneOf"]; conditional {
		for keyword := range schema {
			if keyword != "oneOf" && keyword != "description" {
				return "", fmt.Errorf("%s oneOf cannot be combined with %s", path, keyword)
			}
		}
		var branches []json.RawMessage
		if err := json.Unmarshal(rawOneOf, &branches); err != nil || len(branches) < 2 {
			return "", fmt.Errorf("%s oneOf must contain at least two branches", path)
		}
		commonType := ""
		for index, branch := range branches {
			branchType, err := validateSchemaNode(branch, fmt.Sprintf("%s.oneOf[%d]", path, index))
			if err != nil {
				return "", err
			}
			if commonType == "" {
				commonType = branchType
			} else if commonType != branchType {
				commonType = "any"
			}
		}
		return commonType, nil
	}
	var typ string
	if err := json.Unmarshal(schema["type"], &typ); err != nil || !supported(typ) {
		return "", fmt.Errorf("%s has invalid schema type", path)
	}

	properties := map[string]json.RawMessage{}
	if rawProperties, ok := schema["properties"]; ok {
		if typ != "object" {
			return "", fmt.Errorf("%s properties require object type", path)
		}
		if err := json.Unmarshal(rawProperties, &properties); err != nil || properties == nil {
			return "", fmt.Errorf("%s properties must be an object", path)
		}
		for name, property := range properties {
			if strings.TrimSpace(name) == "" {
				return "", fmt.Errorf("%s property name must not be blank", path)
			}
			if _, err := validateSchemaNode(property, path+"."+name); err != nil {
				return "", err
			}
		}
	}
	if rawAdditional, ok := schema["additionalProperties"]; ok {
		if typ != "object" {
			return "", fmt.Errorf("%s additionalProperties require object type", path)
		}
		var additional bool
		if err := json.Unmarshal(rawAdditional, &additional); err != nil {
			return "", fmt.Errorf("%s additionalProperties must be boolean", path)
		}
	}
	if rawRequired, ok := schema["required"]; ok {
		if typ != "object" {
			return "", fmt.Errorf("%s required fields require object type", path)
		}
		var required []string
		if err := json.Unmarshal(rawRequired, &required); err != nil {
			return "", fmt.Errorf("%s required must be an array", path)
		}
		seen := make(map[string]struct{}, len(required))
		for _, name := range required {
			if strings.TrimSpace(name) == "" {
				return "", fmt.Errorf("%s required name must not be blank", path)
			}
			if _, duplicate := seen[name]; duplicate {
				return "", fmt.Errorf("%s required name is duplicated: %s", path, name)
			}
			seen[name] = struct{}{}
			if _, declared := properties[name]; !declared {
				return "", fmt.Errorf("%s required name not declared: %s", path, name)
			}
		}
	}
	if rawItems, ok := schema["items"]; ok {
		if typ != "array" {
			return "", fmt.Errorf("%s items require array type", path)
		}
		if _, err := validateSchemaNode(rawItems, path+"[]"); err != nil {
			return "", err
		}
	}
	if err := validateIntegerBounds(schema, path, "minLength", "maxLength", typ == "string"); err != nil {
		return "", err
	}
	if err := validateIntegerBounds(schema, path, "minItems", "maxItems", typ == "array"); err != nil {
		return "", err
	}
	if err := validateNumberBounds(schema, path, typ == "number" || typ == "integer"); err != nil {
		return "", err
	}
	if rawEnum, ok := schema["enum"]; ok {
		var values []json.RawMessage
		if err := json.Unmarshal(rawEnum, &values); err != nil || len(values) == 0 {
			return "", fmt.Errorf("%s enum must be a non-empty array", path)
		}
		for _, value := range values {
			if err := validateJSONType(typ, value); err != nil {
				return "", fmt.Errorf("%s enum value does not match %s type", path, typ)
			}
		}
	}
	if rawConst, ok := schema["const"]; ok {
		if err := validateJSONType(typ, rawConst); err != nil {
			return "", fmt.Errorf("%s const value does not match %s type", path, typ)
		}
	}
	return typ, nil
}

func supportedSchemaKeyword(keyword string) bool {
	switch keyword {
	case "type", "description", "enum", "const", "oneOf", "required", "additionalProperties", "properties", "items", "minItems", "maxItems", "minLength", "maxLength", "minimum", "maximum":
		return true
	default:
		return false
	}
}

func validateIntegerBounds(schema map[string]json.RawMessage, path, minName, maxName string, allowed bool) error {
	min, hasMin, err := integerConstraint(schema, minName)
	if err != nil {
		return fmt.Errorf("%s %s must be a non-negative integer", path, minName)
	}
	max, hasMax, err := integerConstraint(schema, maxName)
	if err != nil {
		return fmt.Errorf("%s %s must be a non-negative integer", path, maxName)
	}
	if (hasMin || hasMax) && !allowed {
		return fmt.Errorf("%s %s constraints do not apply to this type", path, minName)
	}
	if hasMin && hasMax && min > max {
		return fmt.Errorf("%s %s exceeds %s", path, minName, maxName)
	}
	return nil
}

func integerConstraint(schema map[string]json.RawMessage, name string) (int, bool, error) {
	raw, ok := schema[name]
	if !ok {
		return 0, false, nil
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil || value < 0 {
		return 0, true, fmt.Errorf("invalid integer constraint")
	}
	return value, true, nil
}

func validateNumberBounds(schema map[string]json.RawMessage, path string, allowed bool) error {
	minimum, hasMinimum, err := numberConstraint(schema, "minimum")
	if err != nil {
		return fmt.Errorf("%s minimum must be a number", path)
	}
	maximum, hasMaximum, err := numberConstraint(schema, "maximum")
	if err != nil {
		return fmt.Errorf("%s maximum must be a number", path)
	}
	if (hasMinimum || hasMaximum) && !allowed {
		return fmt.Errorf("%s numeric constraints do not apply to this type", path)
	}
	if hasMinimum && hasMaximum && minimum > maximum {
		return fmt.Errorf("%s minimum exceeds maximum", path)
	}
	return nil
}

func numberConstraint(schema map[string]json.RawMessage, name string) (float64, bool, error) {
	raw, ok := schema[name]
	if !ok {
		return 0, false, nil
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, true, fmt.Errorf("invalid numeric constraint")
	}
	return value, true, nil
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
	return validateValue("arguments", schema, raw)
}
func validateValue(n string, raw, v json.RawMessage) error {
	var s map[string]json.RawMessage
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("invalid schema for parameter: %s", n)
	}
	if rawOneOf, conditional := s["oneOf"]; conditional {
		var branches []json.RawMessage
		if json.Unmarshal(rawOneOf, &branches) != nil {
			return fmt.Errorf("invalid conditional schema for parameter: %s", n)
		}
		matches := 0
		for _, branch := range branches {
			if validateValue(n, branch, v) == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("parameter must match exactly one schema branch: %s (matched %d)", n, matches)
		}
		return nil
	}
	var t string
	json.Unmarshal(s["type"], &t)
	var x any
	decoder := json.NewDecoder(bytes.NewReader(v))
	decoder.UseNumber()
	if decoder.Decode(&x) != nil {
		return fmt.Errorf("invalid type for parameter: %s", n)
	}
	if err := validateDecodedType(t, x); err != nil {
		return fmt.Errorf("invalid type for parameter: %s", n)
	}
	if rawConst, constrained := s["const"]; constrained && !bytes.Equal(CanonicalJSON(rawConst), CanonicalJSON(v)) {
		return fmt.Errorf("invalid constant value for parameter: %s", n)
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
	if number, ok := x.(json.Number); ok {
		f, err := number.Float64()
		if err != nil {
			return fmt.Errorf("invalid number for parameter: %s", n)
		}
		var min, max float64
		if json.Unmarshal(s["minimum"], &min) == nil && f < min {
			return fmt.Errorf("parameter is below minimum: %s", n)
		}
		if json.Unmarshal(s["maximum"], &max) == nil && f > max {
			return fmt.Errorf("parameter is above maximum: %s", n)
		}
	}
	if object, ok := x.(map[string]any); ok {
		var properties map[string]json.RawMessage
		json.Unmarshal(s["properties"], &properties)
		var required []string
		json.Unmarshal(s["required"], &required)
		for _, name := range required {
			if _, present := object[name]; !present {
				return fmt.Errorf("missing required parameter: %s.%s", n, name)
			}
		}
		additional := true
		if rawAdditional, present := s["additionalProperties"]; present {
			json.Unmarshal(rawAdditional, &additional)
		}
		for name, value := range object {
			propertySchema, declared := properties[name]
			if !declared {
				if !additional {
					return fmt.Errorf("unknown parameter: %s.%s", n, name)
				}
				continue
			}
			encoded, _ := json.Marshal(value)
			if err := validateValue(n+"."+name, propertySchema, encoded); err != nil {
				return err
			}
		}
	}
	if array, ok := x.([]any); ok {
		var minItems, maxItems int
		if json.Unmarshal(s["minItems"], &minItems) == nil && len(array) < minItems {
			return fmt.Errorf("parameter has fewer items than allowed: %s", n)
		}
		if json.Unmarshal(s["maxItems"], &maxItems) == nil && len(array) > maxItems {
			return fmt.Errorf("parameter has more items than allowed: %s", n)
		}
		if itemSchema, present := s["items"]; present {
			for index, item := range array {
				encoded, _ := json.Marshal(item)
				if err := validateValue(fmt.Sprintf("%s[%d]", n, index), itemSchema, encoded); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateJSONType(typ string, raw json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	return validateDecodedType(typ, value)
}

func validateDecodedType(typ string, value any) error {
	valid := false
	switch typ {
	case "any":
		valid = true
	case "string":
		_, valid = value.(string)
	case "boolean":
		_, valid = value.(bool)
	case "number":
		_, valid = value.(json.Number)
	case "integer":
		if number, ok := value.(json.Number); ok {
			f, err := number.Float64()
			valid = err == nil && f == math.Trunc(f)
		}
	case "object":
		_, valid = value.(map[string]any)
	case "array":
		_, valid = value.([]any)
	case "null":
		valid = value == nil
	}
	if !valid {
		return fmt.Errorf("value is not %s", typ)
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
	if t == "" {
		t = "conditional"
	}
	if err := validateValue("result", schema, value); err != nil {
		return fmt.Errorf("tool result does not match declared %s schema", t)
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
