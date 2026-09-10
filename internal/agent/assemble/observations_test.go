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
	if view.Status != "success" || view.ReturnedCount != len(rows) || !view.Truncated || view.ContinuationID == "" {
		t.Fatalf("view metadata=%#v", view)
	}
	var sample struct {
		Head []map[string]any `json:"head"`
		Tail []map[string]any `json:"tail"`
	}
	if err := json.Unmarshal([]byte(view.Summary), &sample); err != nil {
		t.Fatalf("summary is not JSON: %v (%q)", err, view.Summary)
	}
	if len(sample.Head) == 0 || len(sample.Tail) == 0 || sample.Head[0]["index"] != float64(0) || sample.Tail[len(sample.Tail)-1]["index"] != float64(39) {
		t.Fatalf("head/tail sample=%#v", sample)
	}
}

func TestObservationStoreTruncatesUnicodeWithoutBreakingJSON(t *testing.T) {
	result := contract.Result{CallID: "call-unicode", ToolName: "lookup", Content: `"` + strings.Repeat("😀é", 200) + `"`}
	view := NewObservationStore().Store(result, 260)
	encoded := view.JSON()
	if !json.Valid(encoded) || !utf8.ValidString(view.Summary) || len(encoded) > 260 || !view.Truncated {
		t.Fatalf("unicode view bytes=%d validJSON=%v validUTF8=%v view=%#v", len(encoded), json.Valid(encoded), utf8.ValidString(view.Summary), view)
	}
}

func TestObservationStoreTurnsMalformedSuccessIntoTypedErrorView(t *testing.T) {
	store := NewObservationStore()
	view := store.Store(contract.Result{CallID: "call-bad", ToolName: "broken", Content: `{"rows":[`}, 300)
	if view.Status != "error" || !strings.Contains(view.Summary, "INVALID_TOOL_RESULT") || view.Truncated {
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
