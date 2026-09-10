package live

import (
	"context"
	"strings"
	"testing"

	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/turn"
)

func TestSemanticCompletionGateRequiresStructuredAssessment(t *testing.T) {
	client := &scriptedToolClient{responses: []llm.LlmResponse{
		llm.NewLlmResponse(nil, []llm.LlmToolCall{
			llm.NewLlmToolCall("assessment-1", completionAssessmentTool, map[string]any{
				"decision": "CONTINUE",
				"feedback": "Call saturn_list for both rooms and compare the returned users.",
			}),
		}, "tool_calls"),
	}}
	gate, err := NewSemanticCompletionGate(client, "Judge whether the newest request is fully satisfied.")
	if err != nil {
		t.Fatal(err)
	}
	candidate := turn.CompletionCandidate{
		Request:      "how many users overlap between lounge and programming?",
		Answer:       "Let me fetch that data now.",
		Conversation: []llm.LlmMessage{llm.NewLlmMessage("user", "compare the rooms", nil, "")},
		Tools: []turn.ToolCapability{{
			Name:        "saturn_list",
			Description: "L=List room users;Use: inspect current users in a named room;Avoid: do not use for greetings or user activity statistics",
		}},
		Results: []contract.Result{},
	}

	assessment, err := gate.Evaluate(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Decision != turn.CompletionContinue || !strings.Contains(assessment.Feedback, "saturn_list") {
		t.Fatalf("assessment=%#v", assessment)
	}
	if len(client.requests) != 1 || client.requests[0].ToolChoice() != llm.ToolChoiceRequired || len(client.requests[0].Tools()) != 1 {
		t.Fatalf("gate request=%#v", client.requests)
	}
	if client.requests[0].BypassPromptCache() {
		t.Fatal("completion gate unnecessarily bypassed provider prompt caching")
	}
	payload := client.requests[0].Messages()[1].Content()
	for _, expected := range []string{
		candidate.Request,
		candidate.Answer,
		"saturn_list",
		"List room users",
		"do not use for greetings or user activity statistics",
		"compare the rooms",
	} {
		if !strings.Contains(payload, expected) {
			t.Fatalf("gate payload omitted %q: %s", expected, payload)
		}
	}
}

func TestSemanticCompletionGateRejectsUnstructuredProviderResponse(t *testing.T) {
	client := &scriptedToolClient{responses: []llm.LlmResponse{llm.NewLlmResponse("Looks complete.", nil, "stop")}}
	gate, err := NewSemanticCompletionGate(client, "Judge completion.")
	if err != nil {
		t.Fatal(err)
	}

	_, err = gate.Evaluate(context.Background(), turn.CompletionCandidate{Request: "do it", Answer: "I will do it"})
	if err == nil || !strings.Contains(err.Error(), "structured completion assessment") {
		t.Fatalf("error=%v", err)
	}
}
