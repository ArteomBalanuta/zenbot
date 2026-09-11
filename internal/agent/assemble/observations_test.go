package assemble

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"zenbot/internal/agent/tool/contract"
)

func TestObservationStoreRetainsFullResultAndProjectsBoundedHeadTail(t *testing.T) {
	rows := make([]map[string]any, 40)
	for index := range rows {
		rows[index] = map[string]any{
			"index": index,
			"value": fmt.Sprintf("row-%02d-%s", index, strings.Repeat("x", 32)),
		}
	}
	raw, err := json.Marshal(map[string]any{"rows": rows, "source": "database"})
	if err != nil {
		t.Fatal(err)
	}
	result := contract.Result{CallID: "call-large", ToolName: "database_query", Content: string(raw)}
	store := NewObservationStore()
	view := store.Store(result, 420)

	full, found := store.Full("call-large")
	if !found || full.Content != string(raw) {
		t.Fatalf("full result was not retained: found=%v full=%#v", found, full)
	}
	encoded := view.JSON()
	if !json.Valid(encoded) || len(encoded) > 420 {
		t.Fatalf("bounded view bytes=%d valid=%v: %s", len(encoded), json.Valid(encoded), encoded)
	}
	if view.Status != "success" || view.ReturnedCount != len(rows) || !view.Truncated || view.CallID != "call-large" {
		t.Fatalf("view metadata=%#v", view)
	}
	var sample struct {
		Head []map[string]any `json:"head"`
		Tail []map[string]any `json:"tail"`
	}
	if err := json.Unmarshal(view.Data, &sample); err != nil {
		t.Fatalf("sample is not JSON: %v (%q)", err, view.Data)
	}
	if len(sample.Head) == 0 || len(sample.Tail) == 0 || sample.Head[0]["index"] != float64(0) || sample.Tail[len(sample.Tail)-1]["index"] != float64(39) {
		t.Fatalf("head/tail sample=%#v", sample)
	}
}

func TestObservationStoreTruncatesUnicodeWithoutBreakingJSON(t *testing.T) {
	result := contract.Result{CallID: "call-unicode", ToolName: "lookup", Content: `"` + strings.Repeat("😀é", 200) + `"`}
	view := NewObservationStore().Store(result, 260)
	encoded := view.JSON()
	if !json.Valid(encoded) || !utf8.Valid(view.Data) || len(encoded) > 260 || !view.Truncated {
		t.Fatalf("unicode view bytes=%d validJSON=%v validUTF8=%v view=%#v", len(encoded), json.Valid(encoded), utf8.Valid(view.Data), view)
	}
}

func TestObservationStoreTurnsMalformedSuccessIntoTypedErrorView(t *testing.T) {
	store := NewObservationStore()
	view := store.Store(contract.Result{CallID: "call-bad", ToolName: "broken", Content: `{"rows":[`}, 300)
	if view.Status != "error" || view.Code != "INVALID_TOOL_RESULT" || view.Truncated {
		t.Fatalf("malformed success view=%#v", view)
	}
	if !json.Valid(view.JSON()) {
		t.Fatalf("malformed success produced invalid observation JSON: %s", view.JSON())
	}
	full, found := store.Full("call-bad")
	if !found || full.Content != `{"rows":[` {
		t.Fatalf("original malformed result was not retained: found=%v result=%#v", found, full)
	}
}

func TestObservationJSONKeepsStructuredDataAndCommittedReceipts(t *testing.T) {
	result := contract.ActionSuccessResult("sent", "notify", map[string]any{"message": "hello", "target": "alice"}, 1)
	view := NewObservationStore().Store(result, 512)
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(view.JSON(), &envelope); err != nil {
		t.Fatal(err)
	}
	var data map[string]string
	if err := json.Unmarshal(envelope["data"], &data); err != nil || data["target"] != "alice" {
		t.Fatalf("result was not structured data: %s (%v)", view.JSON(), err)
	}
	if string(envelope["effectsCommitted"]) != "true" || string(envelope["deliveryCount"]) != "1" || string(envelope["actionCount"]) != "1" || string(envelope["effectState"]) != `"COMMITTED"` {
		t.Fatalf("committed receipt was lost: %s", view.JSON())
	}
	if _, present := envelope["summary"]; present {
		t.Fatalf("data was double-encoded: %s", view.JSON())
	}
}

func TestObservationJSONKeepsErrorFieldsAndPartialEffects(t *testing.T) {
	result := contract.ErrorResult("partly-sent", "notify", "DELIVERY_FAILED", "The message reached one recipient; the second delivery failed.")
	result.EffectsCommitted, result.DeliveryCount = true, 1
	result.EffectState = contract.EffectPartial
	result.ActionCount = 1
	view := NewObservationStore().Store(result, 512)
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(view.JSON(), &envelope); err != nil {
		t.Fatal(err)
	}
	if string(envelope["code"]) != `"DELIVERY_FAILED"` || string(envelope["effectsCommitted"]) != "true" || string(envelope["deliveryCount"]) != "1" || string(envelope["actionCount"]) != "1" || string(envelope["effectState"]) != `"PARTIAL"` {
		t.Fatalf("error or partial delivery evidence was lost: %s", view.JSON())
	}
	if !strings.Contains(string(envelope["message"]), "second delivery failed") {
		t.Fatalf("error explanation was lost: %s", view.JSON())
	}
}

func TestOversizedObservationReportsOmittedFieldsAndKeepsLaterSmallFacts(t *testing.T) {
	result := contract.SuccessResult("weather", "lookup", map[string]any{
		"large": strings.Repeat("x", 4000), "temperature": 21,
	})
	view := NewObservationStore().Store(result, 512)
	var envelope struct {
		Data          map[string]any `json:"data"`
		OmittedFields []string       `json:"omittedFields"`
		Truncated     bool           `json:"truncated"`
	}
	if err := json.Unmarshal(view.JSON(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Truncated || envelope.Data["temperature"] != float64(21) || len(envelope.OmittedFields) != 1 || envelope.OmittedFields[0] != "large" {
		t.Fatalf("large first field hid later usable facts: %s", view.JSON())
	}
	if len(view.JSON()) > 512 {
		t.Fatalf("observation exceeded its limit: %d", len(view.JSON()))
	}
}

func TestRequiredIndexIncludesBoundedPreviewWithoutChangingReceiptOnlyIndex(t *testing.T) {
	store := NewObservationStore()
	store.Store(contract.SuccessResult("count", "lookup", map[string]any{"count": 0, "large": strings.Repeat("😀", 3000)}), 300)
	store.Store(contract.ErrorResult("error", "lookup", "FAILED", strings.Repeat("é😀\"", 3000)), 300)
	for _, limit := range []int{1024, 512, 256, 0} {
		index := store.IndexWithData(0, limit)
		if len(index) != 2 || index[1].Code != "FAILED" || index[1].Status != "error" {
			t.Fatalf("preview lost receipt identities or errors: %#v", index)
		}
		for _, entry := range index {
			if len(entry.Data)+len(entry.Message) > limit || !entry.Truncated || !utf8.Valid(entry.Data) || !utf8.ValidString(entry.Message) {
				t.Fatalf("preview exceeds %d bytes or lacks valid truncation metadata: %#v", limit, entry)
			}
		}
		if limit >= 256 && !strings.Contains(string(index[0].Data), `"count":0`) {
			t.Fatalf("small zero fact was lost at %d bytes: %s", limit, index[0].Data)
		}
	}
	encoded, err := json.Marshal(store.Index(0))
	if err != nil || strings.Contains(string(encoded), `"data"`) || strings.Contains(string(encoded), `"message"`) {
		t.Fatalf("receipt-only index exposed result payloads: %s (%v)", encoded, err)
	}
}

func TestRequiredIndexPreservesNestedSourceFactsWhenLargeSiblingFieldsAreOmitted(t *testing.T) {
	store := NewObservationStore()
	result := contract.SuccessResult("remote", "saturn_list", map[string]any{
		"messages": []string{strings.Repeat("room roster ", 1000)},
		"data":     map[string]any{"count": 3, "returnedCount": 3, "room": "lounge", "truncated": false, "users": []string{strings.Repeat("a", 2000), "b", "c"}},
	})
	store.Store(result, 10000)
	entries := store.IndexWithData(0, 512)
	var payload struct {
		Data struct {
			Count int    `json:"count"`
			Room  string `json:"room"`
		} `json:"data"`
	}
	if err := json.Unmarshal(entries[0].Data, &payload); err != nil || payload.Data.Count != 3 || payload.Data.Room != "lounge" {
		t.Fatalf("large nested roster displaced source facts: %s (%v)", entries[0].Data, err)
	}
	if !entries[0].Truncated || len(entries[0].OmittedFields) == 0 {
		t.Fatalf("nested preview hides omissions: %#v", entries[0])
	}
	full, _ := store.Full("remote")
	if full.Content != result.Content {
		t.Fatal("nested projection changed source data")
	}
}
