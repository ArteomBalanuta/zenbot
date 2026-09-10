package llm_test

import (
	"encoding/json"
	"testing"
	"zenbot/internal/agent/llm"
)

func TestContractsDefensivelyCopyNestedValues(t *testing.T) {
	args := map[string]any{"nested": map[string]any{"x": 1}}
	call := llm.NewLlmToolCall("id", "fn", args)
	msg := llm.NewLlmMessage("tool", "", []llm.LlmToolCall{call}, "call")
	req := llm.NewLlmRequest([]llm.LlmMessage{msg}, []any{map[string]any{"type": "function"}}, false, nil, nil)
	args["nested"].(map[string]any)["x"] = 2
	got := req.Messages()[0].ToolCalls()[0].Arguments()
	got["nested"].(map[string]any)["x"] = 3
	if args["nested"].(map[string]any)["x"] != 2 || call.Arguments()["nested"].(map[string]any)["x"] != 1 || req.Messages()[0].ToolCalls()[0].Arguments()["nested"].(map[string]any)["x"] != 1 {
		t.Fatal("contract values were not defensively copied")
	}
}

func TestContractsCloneTypedJSONValuesAndRawBytes(t *testing.T) {
	typedMap := map[string]string{"name": "before"}
	typedSlice := []map[string]string{{"value": "before"}}
	raw := json.RawMessage(`{"nested":[1]}`)
	options := map[string]any{"typed": typedMap, "slice": typedSlice, "raw": raw}
	req := llm.NewLlmRequest(nil, []any{options}, false, options, nil)
	typedMap["name"] = "after"
	typedSlice[0]["value"] = "after"
	raw[0] = '['
	got := req.Tools()[0].(map[string]any)
	if got["typed"].(map[string]string)["name"] != "before" || got["slice"].([]map[string]string)[0]["value"] != "before" || string(got["raw"].(json.RawMessage)) != `{"nested":[1]}` {
		t.Fatal("typed JSON values were not cloned")
	}
	accessor := req.ResponseFormat().(map[string]any)
	accessor["typed"].(map[string]string)["name"] = "mutated"
	if req.ResponseFormat().(map[string]any)["typed"].(map[string]string)["name"] != "before" {
		t.Fatal("accessor exposed mutable state")
	}
}

func TestResponseDiagnosticsAreDefensivelyCopied(t *testing.T) {
	diagnostics := map[string]any{"provider": map[string]string{"id": "one"}}
	response := llm.NewLlmResponseWithMetadata("ok", nil, "stop", nil, diagnostics)
	diagnostics["provider"].(map[string]string)["id"] = "changed"
	got := response.ProviderDiagnostics()
	got["provider"].(map[string]string)["id"] = "mutated"
	if response.ProviderDiagnostics()["provider"].(map[string]string)["id"] != "one" {
		t.Fatal("provider diagnostics were not defensively copied")
	}
}
