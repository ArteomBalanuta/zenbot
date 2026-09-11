package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const maxAgentCommandTailBytes = 4000

type AgentArgumentContract interface {
	Schema() json.RawMessage
	Encode(json.RawMessage) (string, error)
}

type AgentExample struct {
	Prompt    string
	Arguments json.RawMessage
}

type AgentToolSpec struct {
	Actionable           bool
	HiddenReason         string
	PrimaryIntent        string
	InternalFallback     bool
	Label                string
	Description          string
	Category             string
	Access               AgentAccess
	Targets              []string
	UseWhen              []string
	WhenNotUse           []string
	Examples             []AgentExample
	Arguments            AgentArgumentContract
	TargetsUser          bool
	RunCommandCompatible bool
	AllowsSilentAction   bool
}

type agentToolOption func(*AgentToolSpec)

func targetedCommand(spec *AgentToolSpec)      { spec.TargetsUser = true }
func runCommandCompatible(spec *AgentToolSpec) { spec.RunCommandCompatible = true }
func allowsSilentAction(spec *AgentToolSpec)   { spec.AllowsSilentAction = true }
func primaryIntent(intent string) agentToolOption {
	return func(spec *AgentToolSpec) { spec.PrimaryIntent = strings.TrimSpace(intent) }
}
func internalAgentFallback(spec *AgentToolSpec) { spec.InternalFallback = true }

func agentTool(label, description, category string, access AgentAccess, arguments AgentArgumentContract, targets []string, useWhen, whenNotUse, examplePrompt, exampleArguments string, options ...agentToolOption) AgentToolSpec {
	spec := AgentToolSpec{
		Actionable:  true,
		Label:       label,
		Description: description,
		Category:    category,
		Access:      access,
		Targets:     append([]string(nil), targets...),
		UseWhen:     []string{useWhen},
		WhenNotUse:  []string{whenNotUse},
		Examples:    []AgentExample{{Prompt: examplePrompt, Arguments: json.RawMessage(exampleArguments)}},
		Arguments:   arguments,
	}
	for _, option := range options {
		option(&spec)
	}
	return spec
}

func hiddenAgentTool(reason string) AgentToolSpec {
	return AgentToolSpec{HiddenReason: reason}
}

func cloneAgentToolSpec(spec AgentToolSpec) AgentToolSpec {
	result := spec
	result.Targets = append([]string(nil), spec.Targets...)
	result.UseWhen = append([]string(nil), spec.UseWhen...)
	result.WhenNotUse = append([]string(nil), spec.WhenNotUse...)
	result.Examples = make([]AgentExample, len(spec.Examples))
	for index, example := range spec.Examples {
		result.Examples[index] = AgentExample{Prompt: example.Prompt, Arguments: cloneJSON(example.Arguments)}
	}
	return result
}

type argumentField struct {
	name        string
	description string
	required    bool
	token       bool
	kind        string
	minimum     *int64
	maximum     *int64
}

func requiredString(name, description string) argumentField {
	return argumentField{name: name, description: description, required: true, kind: "string"}
}

func requiredToken(name, description string) argumentField {
	field := requiredString(name, description)
	field.token = true
	return field
}

func optionalString(name, description string) argumentField {
	return argumentField{name: name, description: description, kind: "string"}
}

func requiredInteger(name, description string, minimum, maximum int64) argumentField {
	return argumentField{
		name: name, description: description, required: true, kind: "integer",
		minimum: &minimum, maximum: &maximum,
	}
}

type emptyArgumentContract struct{ schema json.RawMessage }

func emptyArguments() AgentArgumentContract {
	return emptyArgumentContract{schema: objectSchema(nil, nil)}
}

func (contract emptyArgumentContract) Schema() json.RawMessage { return cloneJSON(contract.schema) }

func (contract emptyArgumentContract) Encode(raw json.RawMessage) (string, error) {
	var arguments struct{}
	if err := decodeStrict(raw, &arguments); err != nil {
		return "", err
	}
	return "", nil
}

type positionalArgumentContract struct {
	fields []argumentField
	schema json.RawMessage
}

func positionalArguments(fields ...argumentField) AgentArgumentContract {
	copied := append([]argumentField(nil), fields...)
	properties := make(map[string]json.RawMessage, len(copied))
	required := make([]string, 0, len(copied))
	for _, field := range copied {
		properties[field.name] = fieldSchema(field)
		if field.required {
			required = append(required, field.name)
		}
	}
	return positionalArgumentContract{fields: copied, schema: objectSchema(properties, required)}
}

func (contract positionalArgumentContract) Schema() json.RawMessage {
	return cloneJSON(contract.schema)
}

func (contract positionalArgumentContract) Encode(raw json.RawMessage) (string, error) {
	arguments, err := decodeObject(raw)
	if err != nil {
		return "", err
	}
	allowed := make(map[string]argumentField, len(contract.fields))
	for _, field := range contract.fields {
		allowed[field.name] = field
	}
	if err := rejectUnknown(arguments, allowed); err != nil {
		return "", err
	}
	parts := make([]string, 0, len(contract.fields))
	for _, field := range contract.fields {
		value, present := arguments[field.name]
		if !present {
			if field.required {
				return "", fmt.Errorf("missing required argument %q", field.name)
			}
			continue
		}
		encoded, err := encodeField(field, value)
		if err != nil {
			return "", err
		}
		if encoded != "" {
			parts = append(parts, encoded)
		}
	}
	return boundedTail(strings.Join(parts, " "))
}

type booleanStateArgumentContract struct {
	field, enabledToken, disabledToken string
	schema                             json.RawMessage
}

func booleanStateArguments(field, description, enabledToken, disabledToken string) AgentArgumentContract {
	property := json.RawMessage(mustJSON(map[string]any{"type": "boolean", "description": description}))
	return booleanStateArgumentContract{
		field: field, enabledToken: enabledToken, disabledToken: disabledToken,
		schema: objectSchema(map[string]json.RawMessage{field: property}, []string{field}),
	}
}

func (contract booleanStateArgumentContract) Schema() json.RawMessage {
	return cloneJSON(contract.schema)
}

func (contract booleanStateArgumentContract) Encode(raw json.RawMessage) (string, error) {
	arguments, err := decodeObject(raw)
	if err != nil {
		return "", err
	}
	if len(arguments) != 1 {
		return "", fmt.Errorf("expected only %q", contract.field)
	}
	var enabled bool
	value, present := arguments[contract.field]
	if !present || json.Unmarshal(value, &enabled) != nil {
		return "", fmt.Errorf("argument %q must be boolean", contract.field)
	}
	if enabled {
		return boundedTail(contract.enabledToken)
	}
	return boundedTail(contract.disabledToken)
}

type commaListArgumentContract struct {
	listField, trailingField string
	allowedTrailing          map[string]struct{}
	schema                   json.RawMessage
}

func commaListArguments(listField, listDescription, trailingField, trailingDescription string, trailingValues []string) AgentArgumentContract {
	allowed := make(map[string]struct{}, len(trailingValues))
	for _, value := range trailingValues {
		allowed[value] = struct{}{}
	}
	items := map[string]any{"type": "string", "minLength": 1}
	properties := map[string]json.RawMessage{
		listField: json.RawMessage(mustJSON(map[string]any{
			"type": "array", "description": listDescription, "items": items, "minItems": 1,
		})),
		trailingField: json.RawMessage(mustJSON(map[string]any{
			"type": "string", "description": trailingDescription, "enum": trailingValues,
		})),
	}
	return commaListArgumentContract{
		listField: listField, trailingField: trailingField, allowedTrailing: allowed,
		schema: objectSchema(properties, []string{listField, trailingField}),
	}
}

func (contract commaListArgumentContract) Schema() json.RawMessage { return cloneJSON(contract.schema) }

func (contract commaListArgumentContract) Encode(raw json.RawMessage) (string, error) {
	arguments, err := decodeObject(raw)
	if err != nil {
		return "", err
	}
	if len(arguments) != 2 {
		return "", fmt.Errorf("expected only %q and %q", contract.listField, contract.trailingField)
	}
	var values []string
	if json.Unmarshal(arguments[contract.listField], &values) != nil || len(values) == 0 {
		return "", fmt.Errorf("argument %q must be a non-empty string array", contract.listField)
	}
	for index, value := range values {
		value = strings.TrimSpace(value)
		if err := validateToken(value); err != nil || strings.Contains(value, ",") {
			return "", fmt.Errorf("invalid %s[%d]", contract.listField, index)
		}
		values[index] = value
	}
	var trailing string
	if json.Unmarshal(arguments[contract.trailingField], &trailing) != nil {
		return "", fmt.Errorf("argument %q must be a string", contract.trailingField)
	}
	if _, allowed := contract.allowedTrailing[trailing]; !allowed {
		return "", fmt.Errorf("invalid %s %q", contract.trailingField, trailing)
	}
	return boundedTail(strings.Join(values, ",") + " " + trailing)
}

type shadowBanArgumentContract struct{ schema json.RawMessage }

func shadowBanArguments() AgentArgumentContract {
	return shadowBanArgumentContract{schema: objectSchema(map[string]json.RawMessage{
		"mode":   enumStringSchema("How the target is matched.", []string{"exact", "contains"}),
		"target": stringSchema("Nickname or containment fragment.", true),
	}, []string{"mode", "target"})}
}

func (contract shadowBanArgumentContract) Schema() json.RawMessage { return cloneJSON(contract.schema) }

func (contract shadowBanArgumentContract) Encode(raw json.RawMessage) (string, error) {
	var arguments struct {
		Mode   string `json:"mode"`
		Target string `json:"target"`
	}
	if err := decodeStrict(raw, &arguments); err != nil {
		return "", err
	}
	arguments.Target = strings.TrimSpace(arguments.Target)
	if err := validateToken(arguments.Target); err != nil {
		return "", fmt.Errorf("invalid target: %w", err)
	}
	switch arguments.Mode {
	case "exact":
		return boundedTail(arguments.Target)
	case "contains":
		return boundedTail("-c " + arguments.Target)
	default:
		return "", fmt.Errorf("unsupported shadow-ban mode %q", arguments.Mode)
	}
}

type automoveArgumentContract struct{ schema json.RawMessage }

func automoveArguments() AgentArgumentContract {
	return automoveArgumentContract{schema: objectSchema(map[string]json.RawMessage{
		"operation":   enumStringSchema("Enable or disable existing auto-move configuration, or configure its rooms.", []string{"enable", "disable", "configure"}),
		"source":      stringSchema("Required only for configure: source room. Omit for enable or disable.", true),
		"destination": stringSchema("Required only for configure: destination room. Omit for enable or disable.", true),
	}, []string{"operation"})}
}

func (contract automoveArgumentContract) Schema() json.RawMessage { return cloneJSON(contract.schema) }

func (contract automoveArgumentContract) Encode(raw json.RawMessage) (string, error) {
	var arguments struct {
		Operation   string `json:"operation"`
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}
	if err := decodeStrict(raw, &arguments); err != nil {
		return "", err
	}
	switch arguments.Operation {
	case "enable":
		if strings.TrimSpace(arguments.Source) != "" || strings.TrimSpace(arguments.Destination) != "" {
			return "", fmt.Errorf("enable does not accept source or destination")
		}
		return "on", nil
	case "disable":
		if strings.TrimSpace(arguments.Source) != "" || strings.TrimSpace(arguments.Destination) != "" {
			return "", fmt.Errorf("disable does not accept source or destination")
		}
		return "off", nil
	case "configure":
		source, destination := strings.TrimSpace(arguments.Source), strings.TrimSpace(arguments.Destination)
		if err := validateToken(source); err != nil {
			return "", fmt.Errorf("invalid source: %w", err)
		}
		if err := validateToken(destination); err != nil {
			return "", fmt.Errorf("invalid destination: %w", err)
		}
		return boundedTail(source + " " + destination)
	default:
		return "", fmt.Errorf("unsupported automove operation %q", arguments.Operation)
	}
}

type notesArgumentContract struct{ schema json.RawMessage }

func notesArguments() AgentArgumentContract {
	return notesArgumentContract{schema: objectSchema(map[string]json.RawMessage{
		"operation": enumStringSchema("List or permanently purge the caller's notes.", []string{"list", "purge"}),
	}, []string{"operation"})}
}

func (contract notesArgumentContract) Schema() json.RawMessage { return cloneJSON(contract.schema) }

func (contract notesArgumentContract) Encode(raw json.RawMessage) (string, error) {
	var arguments struct {
		Operation string `json:"operation"`
	}
	if err := decodeStrict(raw, &arguments); err != nil {
		return "", err
	}
	switch arguments.Operation {
	case "list":
		return "", nil
	case "purge":
		return "purge", nil
	default:
		return "", fmt.Errorf("unsupported notes operation %q", arguments.Operation)
	}
}

func fieldSchema(field argumentField) json.RawMessage {
	if field.kind == "integer" {
		value := map[string]any{"type": "integer", "description": field.description}
		if field.minimum != nil {
			value["minimum"] = *field.minimum
		}
		if field.maximum != nil {
			value["maximum"] = *field.maximum
		}
		return json.RawMessage(mustJSON(value))
	}
	return stringSchema(field.description, field.required)
}

func stringSchema(description string, minLength bool) json.RawMessage {
	value := map[string]any{"type": "string", "description": description}
	if minLength {
		value["minLength"] = 1
	}
	return json.RawMessage(mustJSON(value))
}

func enumStringSchema(description string, values []string) json.RawMessage {
	return json.RawMessage(mustJSON(map[string]any{
		"type": "string", "description": description, "enum": append([]string(nil), values...),
	}))
}

func objectSchema(properties map[string]json.RawMessage, required []string) json.RawMessage {
	if properties == nil {
		properties = map[string]json.RawMessage{}
	}
	value := map[string]any{
		"type": "object", "additionalProperties": false, "properties": properties,
	}
	if len(required) > 0 {
		value["required"] = append([]string(nil), required...)
	}
	return json.RawMessage(mustJSON(value))
}

func encodeField(field argumentField, raw json.RawMessage) (string, error) {
	switch field.kind {
	case "string":
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return "", fmt.Errorf("argument %q must be a string", field.name)
		}
		value = strings.TrimSpace(value)
		if field.required && value == "" {
			return "", fmt.Errorf("argument %q must not be blank", field.name)
		}
		if field.token {
			if err := validateToken(value); err != nil {
				return "", fmt.Errorf("invalid argument %q: %w", field.name, err)
			}
		}
		return value, nil
	case "integer":
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value json.Number
		if err := decoder.Decode(&value); err != nil {
			return "", fmt.Errorf("argument %q must be an integer", field.name)
		}
		parsed, err := strconv.ParseInt(value.String(), 10, 64)
		if err != nil {
			return "", fmt.Errorf("argument %q must be an integer", field.name)
		}
		if field.minimum != nil && parsed < *field.minimum || field.maximum != nil && parsed > *field.maximum {
			return "", fmt.Errorf("argument %q is outside its allowed range", field.name)
		}
		return strconv.FormatInt(parsed, 10), nil
	default:
		return "", fmt.Errorf("unsupported argument type %q", field.kind)
	}
}

func decodeObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var value map[string]json.RawMessage
	if err := decodeStrict(raw, &value); err != nil || value == nil {
		if err == nil {
			err = fmt.Errorf("arguments must be an object")
		}
		return nil, err
	}
	return value, nil
}

func decodeStrict(raw json.RawMessage, target any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("invalid arguments: multiple JSON values")
		}
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func rejectUnknown[T any](arguments map[string]json.RawMessage, allowed map[string]T) error {
	for name := range arguments {
		if _, ok := allowed[name]; !ok {
			return fmt.Errorf("unknown argument %q", name)
		}
	}
	return nil
}

func validateToken(value string) error {
	if value == "" {
		return fmt.Errorf("value must not be blank")
	}
	if strings.IndexFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' ' }) >= 0 {
		return fmt.Errorf("value must be one command token")
	}
	return nil
}

func boundedTail(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > maxAgentCommandTailBytes {
		return "", fmt.Errorf("encoded command arguments exceed %d bytes", maxAgentCommandTailBytes)
	}
	return value, nil
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func cloneJSON(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}
