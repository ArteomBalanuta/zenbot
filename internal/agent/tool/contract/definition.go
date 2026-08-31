package contract

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Access string

const (
	AccessUser      Access = "USER"
	AccessModerator Access = "MODERATOR"
	AccessAdmin     Access = "ADMIN"
)

type Effect string

const (
	ReadOnly Effect = "READ_ONLY"
	Action   Effect = "ACTION"
)

type ResultMode string

const (
	ModelData    ResultMode = "MODEL_DATA"
	RoomDelivery ResultMode = "ROOM_DELIVERY"
)

type Descriptor struct {
	name, label, description, category         string
	access                                     Access
	effect                                     Effect
	mode                                       ResultMode
	parameters, resultSchema                   json.RawMessage
	capabilities, prerequisites, reads, writes []string
	idempotent                                 bool
	timeout                                    time.Duration
	whenNotUse                                 []string
}

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func NewDescriptor(name, label, description, category string, access Access, effect Effect, mode ResultMode, parameters json.RawMessage, capabilities, prerequisites []string, idempotent bool, timeout time.Duration, resultSchema json.RawMessage, reads, writes, whenNotUse []string) (Descriptor, error) {
	if !nameRE.MatchString(name) {
		return Descriptor{}, &ContractError{"invalid name"}
	}
	if strings.TrimSpace(label) == "" || strings.TrimSpace(description) == "" || strings.TrimSpace(category) == "" || len(whenNotUse) == 0 || timeout < 0 {
		return Descriptor{}, &ContractError{"invalid descriptor metadata"}
	}
	if !validAccess(access) || !validEffect(effect) || !validResultMode(mode) {
		return Descriptor{}, &ContractError{"invalid descriptor policy"}
	}
	for _, guidance := range whenNotUse {
		if strings.TrimSpace(guidance) == "" {
			return Descriptor{}, &ContractError{"invalid negative-use guidance"}
		}
	}
	if err := ValidateSchema(parameters, true); err != nil {
		return Descriptor{}, err
	}
	if err := ValidateSchema(resultSchema, false); err != nil {
		return Descriptor{}, err
	}
	return Descriptor{name: name, label: label, description: description, category: category, access: access, effect: effect, mode: mode, parameters: clone(parameters), resultSchema: clone(resultSchema), capabilities: sortedClone(capabilities), prerequisites: sortedClone(prerequisites), reads: sortedClone(reads), writes: sortedClone(writes), idempotent: idempotent, timeout: timeout, whenNotUse: append([]string(nil), whenNotUse...)}, nil
}

type ContractError struct{ Message string }

func (e *ContractError) Error() string { return e.Message }
func sortedClone(s []string) []string {
	r := append([]string(nil), s...)
	sort.Strings(r)
	return r
}
func validAccess(v Access) bool {
	return v == AccessUser || v == AccessModerator || v == AccessAdmin
}
func validEffect(v Effect) bool                     { return v == ReadOnly || v == Action }
func validResultMode(v ResultMode) bool             { return v == ModelData || v == RoomDelivery }
func (d Descriptor) Name() string                   { return d.name }
func (d Descriptor) Label() string                  { return d.label }
func (d Descriptor) Description() string            { return d.description }
func (d Descriptor) Category() string               { return d.category }
func (d Descriptor) Access() Access                 { return d.access }
func (d Descriptor) Effect() Effect                 { return d.effect }
func (d Descriptor) ResultMode() ResultMode         { return d.mode }
func (d Descriptor) Parameters() json.RawMessage    { return clone(d.parameters) }
func (d Descriptor) ResultSchema() json.RawMessage  { return clone(d.resultSchema) }
func (d Descriptor) RequiredCapabilities() []string { return append([]string(nil), d.capabilities...) }
func (d Descriptor) RequiredSuccessfulTools() []string {
	return append([]string(nil), d.prerequisites...)
}
func (d Descriptor) ResourceReads() []string  { return append([]string(nil), d.reads...) }
func (d Descriptor) ResourceWrites() []string { return append([]string(nil), d.writes...) }
func (d Descriptor) Idempotent() bool         { return d.idempotent }
func (d Descriptor) Timeout() time.Duration   { return d.timeout }
func (d Descriptor) IsReadOnly() bool         { return d.effect == ReadOnly }

type Definition struct {
	Name, Description string
	Parameters        json.RawMessage
}

func NewDefinition(d Descriptor) Definition { return Definition{d.name, d.description, d.Parameters()} }
func (d Definition) JSON() json.RawMessage {
	b, _ := json.Marshal(struct {
		Name, Description string
		Parameters        json.RawMessage `json:"parameters"`
	}{d.Name, d.Description, d.Parameters})
	return b
}

type Result struct {
	CallID, ToolName, Content, ErrorCode string
	IsError                              bool
}

func SuccessResult(call, tool string, value any) Result {
	b, _ := json.Marshal(value)
	if s, ok := value.(string); ok {
		b = []byte(s)
	}
	return Result{CallID: call, ToolName: tool, Content: string(b)}
}
func ErrorResult(call, tool, code, msg string) Result {
	if code == "" {
		code = "TOOL_EXECUTION_FAILED"
	}
	return Result{CallID: call, ToolName: tool, Content: msg, ErrorCode: code, IsError: true}
}
func (r Result) Envelope() json.RawMessage {
	if r.IsError {
		return ErrorEnvelope(r.ErrorCode, r.Content)
	}
	return SuccessEnvelope([]byte(r.Content))
}
