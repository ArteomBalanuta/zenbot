package turn

import (
	"fmt"
	"strings"

	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool/contract"
)

type FinalResponseInput struct {
	Response       llm.LlmResponse
	PriorAssistant string
	RequiredTool   string
	Results        []contract.Result
}

type FinalResponseValidator struct{}

func (FinalResponseValidator) Validate(input FinalResponseInput) error {
	if input.Response.FinishReason() == "length" {
		return fmt.Errorf("final response was truncated")
	}
	if len(input.Response.ToolCalls()) != 0 {
		return fmt.Errorf("final response contains tool calls")
	}
	content := strings.TrimSpace(input.Response.Content())
	if content == "" {
		return fmt.Errorf("final response is empty")
	}
	if prior := strings.TrimSpace(input.PriorAssistant); prior != "" && content == prior {
		return fmt.Errorf("final response repeats the prior assistant response")
	}
	if input.RequiredTool != "" {
		for _, result := range input.Results {
			if !result.IsError && result.ToolName == input.RequiredTool && strings.TrimSpace(result.Content) != "" {
				return nil
			}
		}
		return fmt.Errorf("required tool evidence is missing: %s", input.RequiredTool)
	}
	return nil
}
