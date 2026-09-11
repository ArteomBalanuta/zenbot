package live

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/tool/contract"
)

func TestReadToolResultReconstructsOmittedUnicodeDataWithoutRepeatingAction(t *testing.T) {
	store := assemble.NewObservationStore()
	original := contract.ActionSuccessResult("notify-call", "notify", map[string]any{"messages": []string{strings.Repeat("😀é\\\"", 700)}, "deliveredCount": 1}, 1)
	view := store.Store(original, 512)
	if !view.Truncated {
		t.Fatal("fixture must exercise omitted result data")
	}
	reader := NewReadToolResult(store)
	var assembled strings.Builder
	offset := 0
	for {
		args, _ := json.Marshal(map[string]any{"callId": "notify-call", "offset": offset, "limit": 333})
		result, err := reader.Execute(context.Background(), api.Context{}, args)
		if err != nil || result.IsError {
			t.Fatalf("page result=%#v err=%v", result, err)
		}
		if result.EffectsCommitted || result.DeliveryCount != 0 {
			t.Fatal("reading evidence was reported as a new action")
		}
		var page struct {
			Content                        string
			Offset, NextOffset, TotalRunes int
			Done, EffectsCommitted         bool
			DeliveryCount                  int
		}
		if err := json.Unmarshal([]byte(result.Content), &page); err != nil {
			t.Fatal(err)
		}
		if page.Offset != offset || !utf8.ValidString(page.Content) || !page.EffectsCommitted || page.DeliveryCount != 1 {
			t.Fatalf("invalid page or lost original receipt: %#v", page)
		}
		if utf8.RuneCountInString(page.Content) > 333 {
			t.Fatalf("page exceeded requested size: %#v", page)
		}
		projected := assemble.NewObservationStore().Store(result, contract.DefaultMaxModelResultBytes)
		if projected.Truncated {
			t.Fatalf("reader returned a page that is itself truncated: %s", projected.JSON())
		}
		assembled.WriteString(page.Content)
		if page.Done {
			break
		}
		if page.NextOffset <= offset {
			t.Fatalf("pagination did not advance: %#v", page)
		}
		offset = page.NextOffset
	}
	if assembled.String() != original.Content {
		t.Fatal("pages did not reconstruct the exact original result")
	}
}

func TestReadToolResultRecoversErrorObservationSeparatelyFromErrorText(t *testing.T) {
	store := assemble.NewObservationStore()
	original := contract.ActionErrorResult("remote", "saturn_list", "ACTION_OUTCOME_UNKNOWN", "safe failed delivery", contract.EffectUnknown)
	original.ObservedData = json.RawMessage(`{"room":"lounge","count":2,"users":["` + strings.Repeat("😀alice", 1000) + `","bob"]}`)
	store.Store(original, 512)
	reader := NewReadToolResult(store)
	var recovered strings.Builder
	for offset := 0; ; {
		args, _ := json.Marshal(map[string]any{"callId": "remote", "field": "observedData", "offset": offset, "limit": 333})
		result, err := reader.Execute(context.Background(), api.Context{}, args)
		if err != nil || result.IsError || len(result.Content) > resultPageBytes {
			t.Fatalf("page=%+v err=%v", result, err)
		}
		var page struct {
			Content, Status, Code, EffectState string
			DeliveryCount, NextOffset          int
			Done                               bool
		}
		if err := json.Unmarshal([]byte(result.Content), &page); err != nil {
			t.Fatal(err)
		}
		if page.Status != "error" || page.Code != "ACTION_OUTCOME_UNKNOWN" || page.EffectState != "UNKNOWN" || page.DeliveryCount != 0 || result.EffectsCommitted {
			t.Fatalf("receipt=%+v", page)
		}
		recovered.WriteString(page.Content)
		if page.Done {
			break
		}
		if page.NextOffset <= offset {
			t.Fatal("page did not advance")
		}
		offset = page.NextOffset
	}
	if recovered.String() != string(original.ObservedData) {
		t.Fatal("lost full error observation")
	}
	result, err := reader.Execute(context.Background(), api.Context{}, json.RawMessage(`{"callId":"remote"}`))
	if err != nil || result.IsError || !strings.Contains(result.Content, "safe failed delivery") || strings.Contains(result.Content, "alice") {
		t.Fatalf("old error content changed: %+v err=%v", result, err)
	}
}

func TestReadToolResultRejectsUnknownAndOutOfRangePages(t *testing.T) {
	store := assemble.NewObservationStore()
	store.Store(contract.SuccessResult("known", "lookup", map[string]any{"count": 2}), 512)
	reader := NewReadToolResult(store)
	for _, fixture := range []struct{ args, code string }{
		{`{"callId":"missing"}`, "RESULT_NOT_FOUND"},
		{`{"callId":"known","offset":500}`, "INVALID_ARGUMENTS"},
		{`{"callId":"known","offset":-1}`, "INVALID_ARGUMENTS"},
		{`{"callId":"known","limit":100000}`, "INVALID_ARGUMENTS"},
	} {
		result, err := reader.Execute(context.Background(), api.Context{}, json.RawMessage(fixture.args))
		if err != nil || !result.IsError || result.ErrorCode != fixture.code || strings.TrimSpace(result.Content) == "" {
			t.Fatalf("args=%s result=%#v err=%v", fixture.args, result, err)
		}
	}
}

func TestReadToolResultUsesLargePagesAndShrinksEscapedUnicodeToByteBudget(t *testing.T) {
	for _, fixture := range []struct {
		name, text string
		fullPage   bool
	}{
		{"ascii", strings.Repeat("a", 9000), true},
		{"unicode", strings.Repeat("😀", 9000), false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store := assemble.NewObservationStore()
			encoded, _ := json.Marshal(fixture.text)
			store.Store(contract.Result{CallID: "large", ToolName: "lookup", Content: string(encoded)}, 512)
			result, err := NewReadToolResult(store).Execute(context.Background(), api.Context{}, json.RawMessage(`{"callId":"large","limit":6000}`))
			if err != nil || result.IsError {
				t.Fatalf("page=%#v err=%v", result, err)
			}
			var page struct {
				Content    string
				NextOffset int
				Done       bool
			}
			if err := json.Unmarshal([]byte(result.Content), &page); err != nil {
				t.Fatal(err)
			}
			if len(result.Content) > 7000 || page.Done || page.NextOffset != utf8.RuneCountInString(page.Content) {
				t.Fatalf("page violated byte budget or cursor: %d bytes %#v", len(result.Content), page)
			}
			if fixture.fullPage && page.NextOffset != 6000 {
				t.Fatalf("plain text unnecessarily split into tiny pages: %d", page.NextOffset)
			}
			if !fixture.fullPage && (page.NextOffset >= 6000 || page.NextOffset <= 1000) {
				t.Fatalf("Unicode page did not efficiently fit its byte budget: %d", page.NextOffset)
			}
		})
	}
}

func TestReadToolResultCanRecoverOriginalArgumentsAfterIndexTruncation(t *testing.T) {
	store := assemble.NewObservationStore()
	arguments := json.RawMessage(`{"query":"` + strings.Repeat("long query ", 500) + `"}`)
	store.RecordCall("query", "database_sql", arguments)
	store.Store(contract.SuccessResult("query", "database_sql", map[string]any{"rows": []any{}}), 512)
	if receipts := store.Index(0); len(receipts) != 1 || !receipts[0].ArgumentsTruncated {
		t.Fatalf("fixture must omit argument preview: %#v", receipts)
	}
	result, err := NewReadToolResult(store).Execute(context.Background(), api.Context{}, json.RawMessage(`{"callId":"query","field":"arguments","limit":6000}`))
	if err != nil || result.IsError {
		t.Fatalf("arguments page=%#v err=%v", result, err)
	}
	var page struct {
		Field, Content string
		Done           bool
	}
	if err := json.Unmarshal([]byte(result.Content), &page); err != nil {
		t.Fatal(err)
	}
	if page.Field != "arguments" || !page.Done || page.Content != string(arguments) {
		t.Fatalf("original arguments were not recovered: %#v", page)
	}
}
