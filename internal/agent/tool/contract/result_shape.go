package contract

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxResultShapeHintBytes = 512
	maxResultShapeDepth     = 2
	resultShapeOmission     = "[... omitted]"
)

func resultShapeHint(schema json.RawMessage) string {
	shape, ok := projectResultSchema(schema, 0)
	if !ok {
		return ""
	}
	hint := "Result shape (not arguments): " + shape + "."
	if len(hint) <= maxResultShapeHintBytes {
		return hint
	}

	suffix := " " + resultShapeOmission
	end := maxResultShapeHintBytes - len(suffix)
	for end > 0 && !utf8.ValidString(hint[:end]) {
		end--
	}
	return strings.TrimRight(hint[:end], " ,:<{|?") + suffix
}

func projectResultSchema(raw json.RawMessage, depth int) (string, bool) {
	var node map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &node) != nil || node == nil {
		return "", false
	}
	if rawBranches, present := node["oneOf"]; present {
		var branches []json.RawMessage
		if json.Unmarshal(rawBranches, &branches) != nil || len(branches) == 0 {
			return "", false
		}
		shapes := make([]string, 0, len(branches))
		for _, branch := range branches {
			shape, ok := projectResultSchema(branch, depth)
			if !ok {
				return "", false
			}
			shapes = append(shapes, shape)
		}
		sort.Strings(shapes)
		return "oneOf<" + strings.Join(shapes, "|") + ">", true
	}

	var typ string
	if json.Unmarshal(node["type"], &typ) != nil || !supported(typ) {
		return "", false
	}
	switch typ {
	case "object":
		return projectResultObject(node, depth)
	case "array":
		items, present := node["items"]
		if !present {
			return "array", true
		}
		shape, ok := projectResultSchema(items, depth)
		if !ok {
			return "", false
		}
		return "array<" + shape + ">", true
	default:
		return typ, true
	}
}

func projectResultObject(node map[string]json.RawMessage, depth int) (string, bool) {
	var properties map[string]json.RawMessage
	if rawProperties, present := node["properties"]; present {
		if json.Unmarshal(rawProperties, &properties) != nil || properties == nil {
			return "", false
		}
	}
	if len(properties) == 0 {
		return "object", true
	}
	if depth > maxResultShapeDepth {
		return "object{" + resultShapeOmission + "}", true
	}

	required := make(map[string]bool, len(properties))
	if rawRequired, present := node["required"]; present {
		var names []string
		if json.Unmarshal(rawRequired, &names) != nil {
			return "", false
		}
		for _, name := range names {
			required[name] = true
		}
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)

	fields := make([]string, 0, len(names))
	for _, name := range names {
		shape, ok := projectResultSchema(properties[name], depth+1)
		if !ok {
			return "", false
		}
		optional := ""
		if !required[name] {
			optional = "?"
		}
		fields = append(fields, resultShapePropertyName(name)+optional+":"+shape)
	}
	return "{" + strings.Join(fields, ", ") + "}", true
}

func resultShapePropertyName(name string) string {
	if isSimpleResultShapeName(name) {
		return name
	}
	encoded, _ := json.Marshal(name)
	return string(encoded)
}

func isSimpleResultShapeName(name string) bool {
	if name == "" {
		return false
	}
	for index, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_' || (index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}
