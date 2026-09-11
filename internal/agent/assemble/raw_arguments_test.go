package assemble

import (
	"strings"
	"testing"

	"zenbot/internal/agent/llm"
)

func TestProjectionBudgetsRawArgumentsActuallySentToProvider(t *testing.T) {
	for _, raw := range []string{"{broken" + strings.Repeat("x", 20000), `{ "value":` + strings.Repeat(" ", 20000) + `"ok"}`} {
		policy := llm.NewLlmMessage("system", "policy", nil, "")
		request := llm.NewLlmMessage("user", "repair the call", nil, "")
		call := llm.NewLlmToolCall("large", "lookup", raw)
		messages := []Message{policy, request,
			llm.NewLlmMessage("assistant", nil, []llm.LlmToolCall{call}, ""),
			llm.NewLlmMessage("tool", `{"code":"INVALID_ARGUMENTS"}`, nil, "large"),
		}
		wire := messageWireFormat(messages)
		got := wire[2]["tool_calls"].([]map[string]any)[0]["function"].(map[string]any)["arguments"]
		if got != raw {
			t.Fatal("context sizing changed the raw argument representation sent by the provider")
		}
		projection, err := ProjectTurn(messages, nil, nil, ContextInput{
			RequiredPrefix: []Message{policy}, RequiredSuffix: []Message{request}, MaxTokens: 1000,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !projection.Pruned || len(projection.Messages) != 2 {
			t.Fatal("oversized raw argument pair was admitted using decoded argument size")
		}
	}
}
