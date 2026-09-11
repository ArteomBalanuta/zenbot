package live

import (
	"fmt"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
)

func appendRegistryProtocol(messages []llm.LlmMessage, response llm.LlmResponse, batch []toolBatchResult, observations *assemble.ObservationStore, registry *tool.Registry, agent api.Context) ([]llm.LlmMessage, error) {
	calls := response.ToolCalls()
	if len(calls) != len(batch) || observations == nil {
		return messages, fmt.Errorf("agent tool protocol cardinality mismatch")
	}
	// Execution has already completed for the entire batch. Retain every
	// receipt before projection can fail, and append protocol only atomically.
	projected := make([]llm.LlmMessage, 0, len(batch))
	var projectionErr error
	for i, item := range batch {
		if item.Call.ID != calls[i].ID() || item.Result.CallID != item.Call.ID || item.Result.ToolName != item.Call.Name {
			projectionErr = fmt.Errorf("agent tool protocol identity mismatch")
		}
		maxBytes := contract.DefaultMaxModelResultBytes
		if registered, found := registry.Lookup(item.Call.Name); found {
			if descriptor, err := registered.Descriptor(agent); err == nil {
				maxBytes = descriptor.MaxModelResultBytes()
			}
		}
		observations.RecordCall(item.Call.ID, item.Call.Name, item.Call.Arguments)
		view := observations.Store(item.Result, maxBytes)
		encoded := view.JSON()
		if len(encoded) > maxBytes {
			projectionErr = fmt.Errorf("agent tool observation metadata exceeds model result limit")
		}
		projected = append(projected, llm.NewLlmMessage("tool", string(encoded), nil, item.Call.ID))
	}
	if projectionErr != nil {
		return messages, projectionErr
	}
	var content any
	if response.ContentNullable() != nil {
		content = response.Content()
	}
	messages = append(messages, llm.NewLlmMessage("assistant", content, calls, ""))
	messages = append(messages, projected...)
	return messages, nil
}

type toolBatchResult struct {
	Call   execution.Call
	Result contract.Result
}
