package assemble

import (
	"encoding/json"
	"io"
	"sort"
	"strings"
	"sync"

	"zenbot/internal/agent/tool/contract"
)

// ObservationView is a bounded projection. Data stays structured and is always
// marshaled inside the envelope. Use read_tool_result(callId, ...) for omitted data.
type ObservationView struct {
	CallID            string               `json:"callId"`
	Tool              string               `json:"tool"`
	Status            string               `json:"status"`
	Data              json.RawMessage      `json:"data,omitempty"`
	Code              string               `json:"code,omitempty"`
	RelatedCallID     string               `json:"relatedCallId,omitempty"`
	Message           string               `json:"message,omitempty"`
	EffectsCommitted  bool                 `json:"effectsCommitted"`
	EffectState       contract.EffectState `json:"effectState,omitempty"`
	DeliveryCount     int                  `json:"deliveryCount"`
	ActionCount       int                  `json:"actionCount,omitempty"`
	ReturnedCount     int                  `json:"returnedCount,omitempty"`
	Truncated         bool                 `json:"truncated"`
	OmittedFields     []string             `json:"omittedFields,omitempty"`
	OmittedFieldCount int                  `json:"omittedFieldCount,omitempty"`
}

func (view ObservationView) JSON() json.RawMessage {
	encoded, _ := json.Marshal(view)
	return encoded
}

// ObservationStore owns complete request-local evidence independently of its
// disposable provider views and the projected transcript.
type ObservationStore struct {
	mu      sync.RWMutex
	results map[string]contract.Result
	views   map[string]ObservationView
	calls   map[string]observationCall
	order   []string
}

type observationCall struct {
	tool      string
	arguments json.RawMessage
}

// ObservationReceipt contains execution facts and a retrieval identity, never
// a model-generated interpretation of what the user still needs.
type ObservationReceipt struct {
	CallID             string               `json:"callId"`
	Tool               string               `json:"tool"`
	Arguments          json.RawMessage      `json:"arguments,omitempty"`
	ArgumentsTruncated bool                 `json:"argumentsTruncated,omitempty"`
	Status             string               `json:"status"`
	Code               string               `json:"code,omitempty"`
	RelatedCallID      string               `json:"relatedCallId,omitempty"`
	EffectsCommitted   bool                 `json:"effectsCommitted"`
	EffectState        contract.EffectState `json:"effectState,omitempty"`
	DeliveryCount      int                  `json:"deliveryCount"`
	ActionCount        int                  `json:"actionCount,omitempty"`
}

func (store *ObservationStore) RecordCall(callID, toolName string, arguments json.RawMessage) {
	if store == nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.calls == nil {
		store.calls = make(map[string]observationCall)
	}
	store.calls[callID] = observationCall{tool: toolName, arguments: append(json.RawMessage(nil), arguments...)}
}

func (store *ObservationStore) CallArguments(callID string) (json.RawMessage, bool) {
	if store == nil {
		return nil, false
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	call, found := store.calls[callID]
	return append(json.RawMessage(nil), call.arguments...), found
}

// Index preserves every result receipt while bounding argument previews. Full
// result payloads are omitted so they do not duplicate the tool transcript.
func (store *ObservationStore) Index(maxArgumentBytes int) []ObservationReceipt {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	index := make([]ObservationReceipt, 0, len(store.order))
	for _, callID := range store.order {
		view, call := store.views[callID], store.calls[callID]
		receipt := ObservationReceipt{CallID: callID, Tool: view.Tool, Status: view.Status, Code: view.Code, RelatedCallID: view.RelatedCallID, EffectsCommitted: view.EffectsCommitted, EffectState: view.EffectState, DeliveryCount: view.DeliveryCount, ActionCount: view.ActionCount}
		if receipt.Tool == "" {
			receipt.Tool = call.tool
		}
		if len(call.arguments) > 0 {
			if maxArgumentBytes <= 0 {
				receipt.ArgumentsTruncated = true
			} else if len(call.arguments) <= maxArgumentBytes && json.Valid(call.arguments) {
				receipt.Arguments = append(json.RawMessage(nil), call.arguments...)
			} else {
				preview := []rune(string(call.arguments))
				if len(preview) > maxArgumentBytes {
					preview = preview[:maxArgumentBytes]
				}
				for len(preview) > 0 && len(string(preview)) > maxArgumentBytes {
					preview = preview[:len(preview)-1]
				}
				receipt.Arguments, _ = json.Marshal(string(preview))
				receipt.ArgumentsTruncated = len(string(preview)) < len(call.arguments)
			}
		}
		index = append(index, receipt)
	}
	return index
}

func NewObservationStore() *ObservationStore {
	return &ObservationStore{results: make(map[string]contract.Result), views: make(map[string]ObservationView)}
}

func (store *ObservationStore) Store(result contract.Result, maxBytes int) ObservationView {
	view := projectObservation(result, maxBytes)
	if store == nil {
		return view
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.results == nil {
		store.results = make(map[string]contract.Result)
	}
	if store.views == nil {
		store.views = make(map[string]ObservationView)
	}
	if _, exists := store.results[result.CallID]; !exists {
		store.order = append(store.order, result.CallID)
	}
	store.results[result.CallID] = result
	store.views[result.CallID] = cloneObservationView(view)
	return view
}

func (store *ObservationStore) Full(callID string) (contract.Result, bool) {
	if store == nil {
		return contract.Result{}, false
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	result, found := store.results[callID]
	return result, found
}

func (store *ObservationStore) View(callID string) (ObservationView, bool) {
	if store == nil {
		return ObservationView{}, false
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	view, found := store.views[callID]
	return cloneObservationView(view), found
}

func (store *ObservationStore) Views() []ObservationView {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	views := make([]ObservationView, 0, len(store.order))
	for _, callID := range store.order {
		views = append(views, cloneObservationView(store.views[callID]))
	}
	return views
}

func cloneObservationView(view ObservationView) ObservationView {
	view.Data = append(json.RawMessage(nil), view.Data...)
	view.OmittedFields = append([]string(nil), view.OmittedFields...)
	return view
}

func projectObservation(result contract.Result, maxBytes int) ObservationView {
	if maxBytes <= 0 {
		maxBytes = contract.DefaultMaxModelResultBytes
	}
	view := ObservationView{CallID: result.CallID, Tool: result.ToolName, Status: "success", RelatedCallID: result.RelatedCallID, EffectsCommitted: result.EffectsCommitted, EffectState: result.EffectState, DeliveryCount: result.DeliveryCount, ActionCount: result.ActionCount}
	if result.IsError {
		view.Status, view.Code, view.Message = "error", result.ErrorCode, result.Content
		return fitObservationMessage(view, maxBytes)
	}
	value, valid := decodeObservationJSON(result.Content)
	if !valid {
		view.Status, view.Code, view.Message = "error", "INVALID_TOOL_RESULT", "Tool result was not valid JSON."
		return fitObservationMessage(view, maxBytes)
	}
	view.ReturnedCount = observationCount(value)
	view.Data, _ = json.Marshal(value)
	if len(view.JSON()) <= maxBytes {
		return view
	}
	view.Truncated = true
	switch typed := value.(type) {
	case []any:
		view = fitObservationSamples(view, typed, nil, maxBytes)
	case map[string]any:
		if rows, ok := typed["rows"].([]any); ok {
			fields := make(map[string]any, len(typed)-1)
			for key, item := range typed {
				if key != "rows" {
					fields[key] = item
				}
			}
			view = fitObservationSamples(view, rows, fields, maxBytes)
		} else {
			view = fitObservationObject(view, typed, maxBytes)
		}
	case string:
		runes := []rune(typed)
		low, high := 0, len(runes)
		for low < high {
			mid := (low + high + 1) / 2
			view.Data, _ = json.Marshal(string(runes[:mid]))
			if len(view.JSON()) <= maxBytes {
				low = mid
			} else {
				high = mid - 1
			}
		}
		view.Data, _ = json.Marshal(string(runes[:low]))
	default:
		view.Data = nil
	}
	return view
}

func decodeObservationJSON(content string) (any, bool) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, false
	}
	return value, true
}

func observationCount(value any) int {
	switch value := value.(type) {
	case []any:
		return len(value)
	case map[string]any:
		if count, ok := value["returnedCount"].(json.Number); ok {
			if parsed, err := count.Int64(); err == nil && parsed >= 0 && parsed <= int64(^uint(0)>>1) {
				return int(parsed)
			}
		}
		if rows, ok := value["rows"].([]any); ok {
			return len(rows)
		}
	}
	return 0
}

func fitObservationSamples(view ObservationView, rows []any, fields map[string]any, maxBytes int) ObservationView {
	headCount, tailCount := min(4, len(rows)), min(4, len(rows))
	if headCount+tailCount > len(rows) {
		tailCount = len(rows) - headCount
	}
	for {
		sample := make(map[string]any, len(fields)+2)
		for key, value := range fields {
			sample[key] = value
		}
		sample["head"], sample["tail"] = rows[:headCount], rows[len(rows)-tailCount:]
		view.Data, _ = json.Marshal(sample)
		if len(view.JSON()) <= maxBytes {
			return view
		}
		if headCount > 1 || tailCount > 1 {
			if headCount >= tailCount && headCount > 1 {
				headCount--
			} else {
				tailCount--
			}
			continue
		}
		return fitObservationObject(view, sample, maxBytes)
	}
}

// An oversized field must not prevent later, smaller facts from being retained.
func fitObservationObject(view ObservationView, object map[string]any, maxBytes int) ObservationView {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	selected := make(map[string]any, len(keys))
	view.Data = nil
	for _, key := range keys {
		selected[key] = object[key]
		view.Data, _ = json.Marshal(selected)
		if len(view.JSON()) > maxBytes {
			delete(selected, key)
			view.OmittedFields = append(view.OmittedFields, key)
		}
	}
	view.OmittedFieldCount = len(view.OmittedFields)
	view.Data, _ = json.Marshal(selected)
	if len(view.JSON()) > maxBytes {
		view.OmittedFields = nil
	}
	for len(view.JSON()) > maxBytes && len(selected) > 0 {
		largest, size := "", -1
		for key, value := range selected {
			encoded, _ := json.Marshal(value)
			if len(encoded) > size || (len(encoded) == size && key > largest) {
				largest, size = key, len(encoded)
			}
		}
		delete(selected, largest)
		view.OmittedFieldCount++
		view.Data, _ = json.Marshal(selected)
	}
	return view
}

func fitObservationMessage(view ObservationView, maxBytes int) ObservationView {
	if len(view.JSON()) <= maxBytes {
		return view
	}
	view.Truncated = true
	runes := []rune(view.Message)
	low, high := 0, len(runes)
	for low < high {
		mid := (low + high + 1) / 2
		view.Message = string(runes[:mid])
		if len(view.JSON()) <= maxBytes {
			low = mid
		} else {
			high = mid - 1
		}
	}
	view.Message = string(runes[:low])
	return view
}
