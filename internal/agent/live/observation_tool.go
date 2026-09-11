package live

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/tool/contract"
)

const readToolResultName = "read_tool_result"
const resultPageRunes = 6000
const resultPageBytes = 7000

// ReadToolResult exposes only the evidence retained for this invocation. It
// never reruns the original tool, including when that tool performed an action.
type ReadToolResult struct{ store *assemble.ObservationStore }

func NewReadToolResult(store *assemble.ObservationStore) ReadToolResult {
	return ReadToolResult{store: store}
}
func (ReadToolResult) Name() string { return readToolResultName }

func (tool ReadToolResult) Descriptor(api.Context) (contract.Descriptor, error) {
	parameters := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"callId":{"type":"string","minLength":1},"field":{"type":"string","enum":["content","arguments"]},"offset":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1,"maximum":6000}},"required":["callId"]}`)
	return contract.NewDescriptor(tool.Name(), "Read retained tool result", "Read retained evidence by its original callId from an observation or current-turn result index, without rerunning the tool. field defaults to content (original result JSON or error text); use arguments for the original call arguments. offset and limit count Unicode characters, defaulting to 0 and 6000. Pages shrink to fit their byte budget. Concatenate content pages in order using actual nextOffset until done is true. Each page includes the original execution receipt.", "evidence", contract.AccessUser, contract.ReadOnly, contract.ModelData, parameters, nil, nil, true, time.Second, json.RawMessage(`{"type":"object"}`), []string{"retained_tool_results"}, nil, []string{"Use only for retained results from this invocation, especially truncated or pruned observations. Do not rerun actions to recover their outputs."}, contract.WithPrimaryIntent("read_retained_tool_result"))
}

func (tool ReadToolResult) Execute(ctx context.Context, agent api.Context, arguments json.RawMessage) (contract.Result, error) {
	if err := ctx.Err(); err != nil {
		return contract.Result{}, err
	}
	descriptor, err := tool.Descriptor(agent)
	if err != nil {
		return contract.Result{}, err
	}
	if err := contract.ValidateArguments(descriptor.Parameters(), arguments); err != nil {
		return contract.ErrorResult("", tool.Name(), "INVALID_ARGUMENTS", err.Error()), nil
	}
	var input struct {
		CallID string `json:"callId"`
		Field  string `json:"field"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return contract.ErrorResult("", tool.Name(), "INVALID_ARGUMENTS", "callId must be a string; offset and limit must be integers."), nil
	}
	if input.Limit == 0 {
		input.Limit = resultPageRunes
	}
	if input.Field == "" {
		input.Field = "content"
	}
	original, found := tool.store.Full(input.CallID)
	if !found {
		return contract.ErrorResult("", tool.Name(), "RESULT_NOT_FOUND", "No retained result has this callId. Use an original callId from the current-turn result index or a prior tool observation."), nil
	}
	content := original.Content
	if input.Field == "arguments" {
		arguments, found := tool.store.CallArguments(input.CallID)
		if !found {
			return contract.ErrorResult("", tool.Name(), "RESULT_NOT_FOUND", "The original arguments were not retained for this callId."), nil
		}
		content = string(arguments)
	}
	runes := []rune(content)
	if input.Offset > len(runes) {
		return contract.ErrorResult("", tool.Name(), "INVALID_ARGUMENTS", fmt.Sprintf("offset exceeds the result length; use an offset between 0 and %d.", len(runes))), nil
	}
	end := input.Offset + min(input.Limit, len(runes)-input.Offset)
	status := "success"
	if original.IsError {
		status = "error"
	}
	page := struct {
		CallID           string               `json:"callId"`
		Tool             string               `json:"tool"`
		Status           string               `json:"status"`
		Code             string               `json:"code,omitempty"`
		RelatedCallID    string               `json:"relatedCallId,omitempty"`
		EffectsCommitted bool                 `json:"effectsCommitted"`
		EffectState      contract.EffectState `json:"effectState,omitempty"`
		DeliveryCount    int                  `json:"deliveryCount"`
		ActionCount      int                  `json:"actionCount,omitempty"`
		Field            string               `json:"field"`
		Offset           int                  `json:"offset"`
		NextOffset       int                  `json:"nextOffset"`
		TotalRunes       int                  `json:"totalRunes"`
		Done             bool                 `json:"done"`
		Content          string               `json:"content"`
	}{CallID: input.CallID, Tool: original.ToolName, Status: status, Code: original.ErrorCode, RelatedCallID: original.RelatedCallID, EffectsCommitted: original.EffectsCommitted, EffectState: original.EffectState, DeliveryCount: original.DeliveryCount, ActionCount: original.ActionCount, Field: input.Field, Offset: input.Offset, TotalRunes: len(runes)}
	encodePage := func(end int) contract.Result {
		page.NextOffset, page.Done, page.Content = end, end == len(runes), string(runes[input.Offset:end])
		return contract.SuccessResult("", tool.Name(), page)
	}
	result := encodePage(end)
	if len(result.Content) <= resultPageBytes {
		return result, nil
	}
	low, high := input.Offset, end
	for low < high {
		mid := (low + high + 1) / 2
		if len(encodePage(mid).Content) <= resultPageBytes {
			low = mid
		} else {
			high = mid - 1
		}
	}
	result = encodePage(low)
	if len(result.Content) > resultPageBytes || (low == input.Offset && input.Offset < len(runes)) {
		return contract.ErrorResult("", tool.Name(), "RESULT_METADATA_TOO_LARGE", "The retained call metadata exceeds the result page budget."), nil
	}
	return result, nil
}
