package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/observability"
	"zenbot/internal/agent/turn"
)

const (
	completionAssessmentTool       = "submit_completion_assessment"
	completionGateConversationSize = 6
	completionGateMessageChars     = 2000
	completionGateResultChars      = 4000
	completionGateToolChars        = 500
)

// SemanticCompletionGate uses a separate constrained model decision to keep
// fluent but unfinished prose from being mistaken for task completion.
type SemanticCompletionGate struct {
	client       llm.LlmClient
	instructions string
}

func NewSemanticCompletionGate(client llm.LlmClient, instructions string) (*SemanticCompletionGate, error) {
	if client == nil || strings.TrimSpace(instructions) == "" {
		return nil, errors.New("semantic completion gate is incomplete")
	}
	return &SemanticCompletionGate{client: client, instructions: strings.TrimSpace(instructions)}, nil
}

func (g *SemanticCompletionGate) Evaluate(ctx context.Context, candidate turn.CompletionCandidate) (turn.CompletionAssessment, error) {
	if g == nil || g.client == nil || strings.TrimSpace(g.instructions) == "" {
		return turn.CompletionAssessment{}, errors.New("semantic completion gate is incomplete")
	}
	if err := ctx.Err(); err != nil {
		return turn.CompletionAssessment{}, err
	}
	payload, err := json.Marshal(completionGatePayload(candidate))
	if err != nil {
		return turn.CompletionAssessment{}, fmt.Errorf("encode completion candidate: %w", err)
	}
	request := llm.NewLlmRequest(
		[]llm.LlmMessage{
			llm.NewLlmMessage("system", g.instructions, nil, ""),
			llm.NewLlmMessage("user", "COMPLETION_CANDIDATE_JSON="+string(payload), nil, ""),
		},
		[]any{completionAssessmentDefinition()},
		false,
		nil,
		nil,
	).WithToolChoice(llm.ToolChoiceRequired)
	response, err := g.client.Complete(observability.WithStage(ctx, "llm.semantic_completion_gate"), request)
	if err != nil {
		return turn.CompletionAssessment{}, fmt.Errorf("evaluate semantic completion: %w", err)
	}
	return parseCompletionAssessment(response)
}

func completionAssessmentDefinition() any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        completionAssessmentTool,
			"description": "Submit the semantic completion decision for the supplied candidate. This does not execute a Saturn action.",
			"parameters": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"decision": map[string]any{"type": "string", "enum": []string{string(turn.CompletionFinal), string(turn.CompletionContinue)}},
					"feedback": map[string]any{"type": "string", "minLength": 1},
				},
				"required": []string{"decision", "feedback"},
			},
		},
	}
}

func parseCompletionAssessment(response llm.LlmResponse) (turn.CompletionAssessment, error) {
	if response.FinishReason() == "length" {
		return turn.CompletionAssessment{}, errors.New("structured completion assessment was truncated")
	}
	calls := response.ToolCalls()
	if len(calls) != 1 || calls[0].Name() != completionAssessmentTool {
		return turn.CompletionAssessment{}, errors.New("provider did not return one structured completion assessment")
	}
	arguments := calls[0].Arguments()
	if len(arguments) != 2 {
		return turn.CompletionAssessment{}, errors.New("structured completion assessment has invalid fields")
	}
	decision, decisionOK := arguments["decision"].(string)
	feedback, feedbackOK := arguments["feedback"].(string)
	assessment := turn.CompletionAssessment{Decision: turn.CompletionDecision(strings.ToUpper(strings.TrimSpace(decision))), Feedback: strings.TrimSpace(feedback)}
	if !decisionOK || !feedbackOK || assessment.Feedback == "" || (assessment.Decision != turn.CompletionFinal && assessment.Decision != turn.CompletionContinue) {
		return turn.CompletionAssessment{}, errors.New("structured completion assessment is invalid")
	}
	return assessment, nil
}

type gatePayload struct {
	NewestRequest  string                `json:"newestRequest"`
	Candidate      string                `json:"candidateAnswer"`
	CanContinue    bool                  `json:"canContinue"`
	Conversation   []gateMessage         `json:"recentConversation"`
	AvailableTools []turn.ToolCapability `json:"availableTools"`
	Observations   []gateObservation     `json:"toolObservations"`
}

type gateMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type gateObservation struct {
	Tool      string `json:"tool"`
	Status    string `json:"status"`
	Content   string `json:"content"`
	ErrorCode string `json:"errorCode,omitempty"`
}

func completionGatePayload(candidate turn.CompletionCandidate) gatePayload {
	tools := make([]turn.ToolCapability, 0, len(candidate.Tools))
	seenTools := make(map[string]struct{}, len(candidate.Tools))
	for _, capability := range candidate.Tools {
		name := strings.TrimSpace(capability.Name)
		if name == "" {
			continue
		}
		if _, found := seenTools[name]; found {
			continue
		}
		seenTools[name] = struct{}{}
		tools = append(tools, turn.ToolCapability{
			Name:        name,
			Description: truncateText(strings.TrimSpace(capability.Description), completionGateToolChars),
		})
	}
	observations := make([]gateObservation, 0, len(candidate.Results))
	for _, result := range candidate.Results {
		status := "success"
		if result.IsError {
			status = "error"
		}
		observations = append(observations, gateObservation{
			Tool:      result.ToolName,
			Status:    status,
			Content:   truncateText(result.Content, completionGateResultChars),
			ErrorCode: result.ErrorCode,
		})
	}
	return gatePayload{
		NewestRequest:  strings.TrimSpace(candidate.Request),
		Candidate:      strings.TrimSpace(candidate.Answer),
		CanContinue:    candidate.CanContinue,
		Conversation:   completionGateConversation(candidate.Conversation),
		AvailableTools: tools,
		Observations:   observations,
	}
}

func completionGateConversation(messages []llm.LlmMessage) []gateMessage {
	conversation := make([]gateMessage, 0, completionGateConversationSize)
	for _, message := range messages {
		content := strings.TrimSpace(message.Content())
		if (message.Role() != "user" && message.Role() != "assistant") || content == "" || len(message.ToolCalls()) != 0 || isBulkRetryContext(content) {
			continue
		}
		conversation = append(conversation, gateMessage{Role: message.Role(), Content: truncateText(content, completionGateMessageChars)})
	}
	if len(conversation) > completionGateConversationSize {
		conversation = conversation[len(conversation)-completionGateConversationSize:]
	}
	return conversation
}

func truncateText(value string, limit int) string {
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func completionToolCapabilities(providerTools []any) []turn.ToolCapability {
	capabilities := make([]turn.ToolCapability, 0, len(providerTools))
	for _, raw := range providerTools {
		definition, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		function, ok := definition["function"].(map[string]any)
		if !ok {
			continue
		}
		name, _ := function["name"].(string)
		description, _ := function["description"].(string)
		if strings.TrimSpace(name) != "" {
			capabilities = append(capabilities, turn.ToolCapability{Name: name, Description: description})
		}
	}
	return capabilities
}
