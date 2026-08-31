package llm

import (
	"context"
	"encoding/json"
	"reflect"
)

// LlmMessage is an immutable provider-neutral chat message. A nil content is
// preserved for assistant tool-call messages.
type LlmMessage struct {
	role       string
	content    *string
	toolCallID *string
	toolCalls  []LlmToolCall
}

// NewLlmMessage accepts either a string or nil for content.
func NewLlmMessage(role string, content any, toolCalls []LlmToolCall, toolCallID string) LlmMessage {
	var text *string
	if content != nil {
		v, ok := content.(string)
		if !ok {
			panic("llm message content must be string or nil")
		}
		text = &v
	}
	var id *string
	if toolCallID != "" {
		v := toolCallID
		id = &v
	}
	return LlmMessage{role: role, content: text, toolCallID: id, toolCalls: cloneToolCalls(toolCalls)}
}
func (m LlmMessage) Role() string { return m.role }
func (m LlmMessage) Content() string {
	if m.content == nil {
		return ""
	}
	return *m.content
}
func (m LlmMessage) ContentNullable() *string {
	if m.content == nil {
		return nil
	}
	v := *m.content
	return &v
}
func (m LlmMessage) ToolCallID() string {
	if m.toolCallID == nil {
		return ""
	}
	return *m.toolCallID
}
func (m LlmMessage) ToolCallIDNullable() *string {
	if m.toolCallID == nil {
		return nil
	}
	v := *m.toolCallID
	return &v
}
func (m LlmMessage) ToolCalls() []LlmToolCall { return cloneToolCalls(m.toolCalls) }

// LlmToolCall carries raw JSON arguments while retaining a convenient decoded map.
type LlmToolCall struct {
	id, name     string
	arguments    map[string]any
	rawArguments string
}

func NewLlmToolCall(id, name string, arguments any) LlmToolCall {
	var m map[string]any
	var raw string
	switch v := arguments.(type) {
	case map[string]any:
		m = cloneMap(v)
		b, _ := json.Marshal(v)
		raw = string(b)
	case string:
		raw = v
		_ = json.Unmarshal([]byte(v), &m)
	case nil:
		raw = ""
	default:
		panic("llm tool arguments must be JSON string or object")
	}
	return LlmToolCall{id: id, name: name, arguments: m, rawArguments: raw}
}
func (t LlmToolCall) ID() string                { return t.id }
func (t LlmToolCall) Name() string              { return t.name }
func (t LlmToolCall) Arguments() map[string]any { return cloneMap(t.arguments) }
func (t LlmToolCall) RawArguments() string      { return t.rawArguments }

// LlmRequest is an immutable completion request.
type LlmRequest struct {
	messages          []LlmMessage
	tools             []any
	bypassPromptCache bool
	responseFormat    any
	projection        any
}

func NewLlmRequest(messages []LlmMessage, tools []any, bypassPromptCache bool, responseFormat, projection any) LlmRequest {
	return LlmRequest{messages: cloneMessages(messages), tools: cloneAnySlice(tools), bypassPromptCache: bypassPromptCache, responseFormat: cloneValue(responseFormat), projection: cloneValue(projection)}
}
func (r LlmRequest) Messages() []LlmMessage  { return cloneMessages(r.messages) }
func (r LlmRequest) Tools() []any            { return cloneAnySlice(r.tools) }
func (r LlmRequest) BypassPromptCache() bool { return r.bypassPromptCache }
func (r LlmRequest) ResponseFormat() any     { return cloneValue(r.responseFormat) }
func (r LlmRequest) Projection() any         { return cloneValue(r.projection) }

type LlmResponse struct {
	content             *string
	finishReason        string
	toolCalls           []LlmToolCall
	usage               map[string]int
	providerDiagnostics map[string]any
}

func NewLlmResponse(content any, toolCalls []LlmToolCall, finishReason string) LlmResponse {
	return NewLlmResponseWithMetadata(content, toolCalls, finishReason, nil, nil)
}
func NewLlmResponseWithMetadata(content any, toolCalls []LlmToolCall, finishReason string, usage map[string]int, diagnostics map[string]any) LlmResponse {
	var p *string
	if content != nil {
		v, ok := content.(string)
		if !ok {
			panic("llm response content must be string or nil")
		}
		p = &v
	}
	return LlmResponse{content: p, toolCalls: cloneToolCalls(toolCalls), finishReason: finishReason, usage: cloneIntMap(usage), providerDiagnostics: cloneMap(diagnostics)}
}
func (r LlmResponse) Content() string {
	if r.content == nil {
		return ""
	}
	return *r.content
}
func (r LlmResponse) ContentNullable() *string {
	if r.content == nil {
		return nil
	}
	v := *r.content
	return &v
}
func (r LlmResponse) ToolCalls() []LlmToolCall            { return cloneToolCalls(r.toolCalls) }
func (r LlmResponse) FinishReason() string                { return r.finishReason }
func (r LlmResponse) Usage() map[string]int               { return cloneIntMap(r.usage) }
func (r LlmResponse) ProviderDiagnostics() map[string]any { return cloneMap(r.providerDiagnostics) }

type LlmClient interface {
	Complete(context.Context, LlmRequest) (LlmResponse, error)
}

type LlmError struct {
	Code            string
	Status          int
	ProviderCode    string
	ProviderMessage string
	Snippet         string
	Err             error
}

func (e *LlmError) Error() string {
	s := e.Code
	if e.Status != 0 {
		s += ": upstream status " + itoa(e.Status)
	}
	if e.ProviderCode != "" {
		s += ": " + e.ProviderCode
	}
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}
func (e *LlmError) Unwrap() error { return e.Err }
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	b := []byte{}
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

func cloneMessages(in []LlmMessage) []LlmMessage {
	out := make([]LlmMessage, len(in))
	for i, m := range in {
		out[i] = NewLlmMessage(m.role, m.contentNullableValue(), m.toolCalls, m.ToolCallID())
	}
	return out
}
func (m LlmMessage) contentNullableValue() any {
	if m.content == nil {
		return nil
	}
	return *m.content
}
func cloneToolCalls(in []LlmToolCall) []LlmToolCall {
	out := make([]LlmToolCall, len(in))
	for i, t := range in {
		out[i] = LlmToolCall{id: t.id, name: t.name, arguments: cloneMap(t.arguments), rawArguments: t.rawArguments}
	}
	return out
}
func cloneAnySlice(in []any) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = cloneValue(v)
	}
	return out
}
func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneValue(v)
	}
	return out
}
func cloneIntMap(in map[string]int) map[string]int {
	if in == nil {
		return nil
	}
	out := map[string]int{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

// CloneJSONValue defensively copies JSON-compatible values while preserving their Go types.
func CloneJSONValue(v any) any {
	if v == nil {
		return nil
	}
	return cloneReflect(reflect.ValueOf(v)).Interface()
}

func cloneValue(v any) any {
	if v == nil {
		return nil
	}
	return CloneJSONValue(v)
}

func cloneReflect(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return reflect.Value{}
	}
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		cloned := cloneReflect(v.Elem())
		out := reflect.New(v.Type()).Elem()
		if cloned.IsValid() && cloned.Type().AssignableTo(v.Type()) {
			out.Set(cloned)
		} else {
			out.Set(cloned)
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), cloneReflect(iter.Value()))
		}
		return out
	case reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(cloneReflect(v.Index(i)))
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(cloneReflect(v.Index(i)))
		}
		return out
	case reflect.Pointer:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(cloneReflect(v.Elem()))
		return out
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath == "" {
				out.Field(i).Set(cloneReflect(v.Field(i)))
			}
		}
		return out
	default:
		return v
	}
}
