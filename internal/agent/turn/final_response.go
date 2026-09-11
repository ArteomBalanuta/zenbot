package turn

import (
	"fmt"
	"strings"

	"zenbot/internal/agent/llm"
)

type FinalResponseInput struct {
	Response llm.LlmResponse
}

type FinalResponseValidator struct{}

// ContainsToolProtocolArtifact reports provider-native tool syntax that must
// never be rendered as a user-facing answer.
func ContainsToolProtocolArtifact(content string) bool {
	normalized := strings.ToLower(content)
	return strings.Contains(normalized, "<|tool_call") || strings.Contains(normalized, "<tool_call")
}

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
	if ContainsToolProtocolArtifact(content) {
		return fmt.Errorf("final response contains textual tool protocol markup")
	}
	return nil
}
