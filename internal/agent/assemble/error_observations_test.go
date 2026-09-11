package assemble

import (
	"encoding/json"
	"strings"
	"testing"

	"zenbot/internal/agent/tool/contract"
)

func TestErrorObservationKeepsBoundedFactsAndImmutableFullEvidence(t *testing.T) {
	result := contract.ActionErrorResult("remote", "saturn_list", "ACTION_OUTCOME_UNKNOWN", strings.Repeat("safe delivery failure 😀 ", 1000), contract.EffectUnknown)
	result.ActionCount = 2
	result.EffectsCommitted = true
	result.ObservedData = json.RawMessage(`{"messages":[],"deliveredCount":0,"actionCount":2,"data":{"room":"lounge","count":2,"returnedCount":2,"users":["` + strings.Repeat("a", 5000) + `","bob"],"truncated":false}}`)
	want := string(result.ObservedData)
	store := NewObservationStore()
	view := store.Store(result, 900)
	if !json.Valid(view.JSON()) || len(view.JSON()) > 900 || !view.Truncated || view.Status != "error" || view.Code != "ACTION_OUTCOME_UNKNOWN" || view.EffectState != contract.EffectUnknown || view.DeliveryCount != 0 || view.ActionCount != 2 || view.Message == "" {
		t.Fatalf("bounded error=%s", view.JSON())
	}
	if !strings.Contains(string(view.Data), `"room":"lounge"`) || !strings.Contains(string(view.Data), `"count":2`) {
		t.Fatalf("facts displaced by error: %s", view.JSON())
	}
	result.ObservedData[0] = 'x'
	full, found := store.Full("remote")
	if !found || string(full.ObservedData) != want {
		t.Fatal("caller mutated stored evidence")
	}
	full.ObservedData[0] = 'x'
	full, _ = store.Full("remote")
	if string(full.ObservedData) != want {
		t.Fatal("retrieval mutated stored evidence")
	}
	index := store.IndexWithData(0, 900)
	if len(index) != 1 || index[0].Status != "error" || index[0].Code != "ACTION_OUTCOME_UNKNOWN" || index[0].DeliveryCount != 0 || !strings.Contains(string(index[0].Data), `"count":2`) {
		t.Fatalf("index=%+v", index)
	}
	if receipt := store.Index(0)[0]; receipt.EffectState != contract.EffectUnknown || receipt.ActionCount != 2 {
		t.Fatalf("receipt=%+v", receipt)
	}
}

func TestMalformedOptionalErrorObservationKeepsOriginalFailure(t *testing.T) {
	result := contract.ActionErrorResult("bad", "lookup", "ACTION_OUTCOME_UNKNOWN", "safe message", contract.EffectUnknown)
	result.ObservedData = json.RawMessage(`{"private":`)
	view := NewObservationStore().Store(result, 512)
	if view.Code != "ACTION_OUTCOME_UNKNOWN" || view.Message != "safe message" || len(view.Data) != 0 || !json.Valid(view.JSON()) {
		t.Fatalf("view=%s", view.JSON())
	}
}

func TestErrorObservationDoesNotTruncateWhenErrorAndDataFit(t *testing.T) {
	result := contract.ErrorResult("remote", "saturn_list", "FAILED", strings.Repeat("safe failure ", 50))
	result.ObservedData = json.RawMessage(`{"room":"lounge","count":2}`)
	view := NewObservationStore().Store(result, 2000)
	if view.Truncated || view.Message != result.Content || !strings.Contains(string(view.Data), `"count":2`) {
		t.Fatalf("fitting observation was truncated: %s", view.JSON())
	}
}
