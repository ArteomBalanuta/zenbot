package assemble

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"sync"

	"zenbot/internal/agent/tool/contract"
)

// ObservationView is the bounded, provider-visible representation of one tool
// result. Summary is text rather than RawMessage so untrusted result data can
// never escape the surrounding JSON envelope.
type ObservationView struct {
	CallID         string `json:"callId"`
	Tool           string `json:"tool"`
	Status         string `json:"status"`
	Summary        string `json:"summary"`
	ReturnedCount  int    `json:"returnedCount"`
	Truncated      bool   `json:"truncated"`
	ContinuationID string `json:"continuationId,omitempty"`
}

func (view ObservationView) JSON() json.RawMessage {
	encoded, _ := json.Marshal(view)
	return encoded
}

// ObservationStore owns complete request-local results. Only ObservationView
// values are suitable for insertion into a provider transcript.
type ObservationStore struct {
	mu      sync.RWMutex
	results map[string]contract.Result
	views   map[string]ObservationView
	order   []string
}

func NewObservationStore() *ObservationStore {
	return &ObservationStore{results: make(map[string]contract.Result), views: make(map[string]ObservationView)}
}

func (store *ObservationStore) Store(result contract.Result, maxBytes int) ObservationView {
	if store == nil {
		store = NewObservationStore()
	}
	view := projectObservation(result, maxBytes)
	store.mu.Lock()
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
	store.views[result.CallID] = view
	store.mu.Unlock()
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
	return view, found
}

func (store *ObservationStore) Views() []ObservationView {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	views := make([]ObservationView, 0, len(store.order))
	for _, callID := range store.order {
		views = append(views, store.views[callID])
	}
	return views
}

func projectObservation(result contract.Result, maxBytes int) ObservationView {
	if maxBytes <= 0 {
		maxBytes = 8192
	}
	view := ObservationView{CallID: result.CallID, Tool: result.ToolName, Status: "success"}
	if result.IsError {
		view.Status = "error"
		view.Summary = strings.TrimSpace(result.ErrorCode + ": " + result.Content)
		return fitObservationText(view, maxBytes)
	}
	value, valid := decodeObservationJSON(result.Content)
	if !valid {
		view.Status = "error"
		view.Summary = "INVALID_TOOL_RESULT: tool result was not valid JSON"
		return fitObservationText(view, maxBytes)
	}
	return fitObservationValue(view, value, maxBytes)
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

func fitObservationValue(view ObservationView, value any, maxBytes int) ObservationView {
	complete, _ := json.Marshal(value)
	view.Summary = string(complete)
	if len(view.JSON()) <= maxBytes {
		return view
	}
	view.Truncated = true
	view.ContinuationID = observationContinuationID(view.CallID)
	switch typed := value.(type) {
	case []any:
		view.ReturnedCount = len(typed)
		return fitObservationSamples(view, typed, nil, maxBytes)
	case map[string]any:
		if rows, ok := typed["rows"].([]any); ok {
			view.ReturnedCount = len(rows)
			fields := make(map[string]any, len(typed)-1)
			for key, item := range typed {
				if key != "rows" {
					fields[key] = item
				}
			}
			return fitObservationSamples(view, rows, fields, maxBytes)
		}
		return fitObservationObject(view, typed, maxBytes)
	case string:
		return fitObservationTextValue(view, typed, maxBytes)
	default:
		return fitObservationText(view, maxBytes)
	}
}

func fitObservationSamples(view ObservationView, rows []any, fields map[string]any, maxBytes int) ObservationView {
	headCount, tailCount := minInt(4, len(rows)), minInt(4, len(rows))
	if headCount+tailCount > len(rows) {
		tailCount = len(rows) - headCount
	}
	for {
		summary := make(map[string]any, len(fields)+2)
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			summary[key] = fields[key]
		}
		summary["head"] = append([]any(nil), rows[:headCount]...)
		summary["tail"] = append([]any(nil), rows[len(rows)-tailCount:]...)
		encoded, _ := json.Marshal(summary)
		view.Summary = string(encoded)
		if len(view.JSON()) <= maxBytes {
			return view
		}
		if headCount > 1 || tailCount > 1 {
			if headCount >= tailCount && headCount > 1 {
				headCount--
			} else if tailCount > 1 {
				tailCount--
			}
			continue
		}
		if len(fields) > 0 {
			delete(fields, keys[len(keys)-1])
			continue
		}
		return fitObservationText(view, maxBytes)
	}
}

func fitObservationObject(view ObservationView, object map[string]any, maxBytes int) ObservationView {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	selected := make(map[string]any, len(keys))
	for _, key := range keys {
		selected[key] = object[key]
		encoded, _ := json.Marshal(selected)
		view.Summary = string(encoded)
		if len(view.JSON()) > maxBytes {
			delete(selected, key)
			break
		}
	}
	for len(selected) > 0 {
		encoded, _ := json.Marshal(selected)
		view.Summary = string(encoded)
		if len(view.JSON()) <= maxBytes {
			return view
		}
		for index := len(keys) - 1; index >= 0; index-- {
			if _, found := selected[keys[index]]; found {
				delete(selected, keys[index])
				break
			}
		}
	}
	return fitObservationText(view, maxBytes)
}

func fitObservationTextValue(view ObservationView, value string, maxBytes int) ObservationView {
	runes := []rune(value)
	low, high := 0, len(runes)
	for low < high {
		mid := (low + high + 1) / 2
		view.Summary = string(runes[:mid])
		if len(view.JSON()) <= maxBytes {
			low = mid
		} else {
			high = mid - 1
		}
	}
	view.Summary = string(runes[:low])
	return fitObservationText(view, maxBytes)
}

func fitObservationText(view ObservationView, maxBytes int) ObservationView {
	if len(view.JSON()) <= maxBytes {
		return view
	}
	view.Truncated = true
	if view.ContinuationID == "" {
		view.ContinuationID = observationContinuationID(view.CallID)
	}
	runes := []rune(view.Summary)
	for len(runes) > 0 && len(view.JSON()) > maxBytes {
		runes = runes[:len(runes)-1]
		view.Summary = string(runes)
	}
	return view
}

func observationContinuationID(callID string) string {
	digest := sha256.Sum256([]byte(callID))
	return hex.EncodeToString(digest[:8])
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
