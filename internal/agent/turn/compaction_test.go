package turn

import (
	"context"
	"strings"
	"testing"

	"zenbot/internal/agent/llm"
)

type recordingSummarizer struct{ received []llm.LlmMessage }

func (s *recordingSummarizer) Summarize(_ context.Context, messages []llm.LlmMessage) (string, error) {
	s.received = append([]llm.LlmMessage(nil), messages...)
	return "older conversation summary", nil
}

func TestMemoryCompactorSummarizesOnlyContiguousOldestCompleteTurns(t *testing.T) {
	messages := []llm.LlmMessage{
		llm.NewLlmMessage("user", "u1", nil, ""), llm.NewLlmMessage("assistant", "a1", nil, ""),
		llm.NewLlmMessage("user", "u2", nil, ""), llm.NewLlmMessage("assistant", "a2", nil, ""),
		llm.NewLlmMessage("user", "u3", nil, ""), llm.NewLlmMessage("assistant", "a3", nil, ""),
	}
	summarizer := &recordingSummarizer{}
	result, err := (MemoryCompactor{Summarizer: summarizer}).Compact(context.Background(), CompactionInput{Messages: messages, RawTurnLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(summarizer.received) != 4 || len(result.Messages) != 3 {
		t.Fatalf("summarized=%#v projected=%#v", summarizer.received, result.Messages)
	}
	if result.Summary == nil || result.Summary.CoveredFrom != 0 || result.Summary.CoveredThrough != 3 || result.Summary.Fingerprint == "" {
		t.Fatalf("summary metadata=%#v", result.Summary)
	}
	if !strings.Contains(result.Messages[0].Content(), "older conversation summary") || result.Messages[1].Content() != "u3" || result.Messages[2].Content() != "a3" {
		t.Fatalf("projected messages=%#v", result.Messages)
	}
}

func TestMemoryCompactorReplacesPriorSummaryAndKeepsRawAuthority(t *testing.T) {
	messages := []llm.LlmMessage{
		llm.NewLlmMessage("user", "u1", nil, ""), llm.NewLlmMessage("assistant", "a1", nil, ""),
		llm.NewLlmMessage("user", "u2", nil, ""), llm.NewLlmMessage("assistant", "a2", nil, ""),
	}
	prior := &MemorySummary{Content: "prior", CoveredFrom: -4, CoveredThrough: -1, Fingerprint: "old"}
	summarizer := &recordingSummarizer{}
	result, err := (MemoryCompactor{Summarizer: summarizer}).Compact(context.Background(), CompactionInput{Messages: messages, RawTurnLimit: 1, ExistingSummary: prior})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 || messages[0].Content() != "u1" {
		t.Fatal("raw messages were mutated")
	}
	if result.Summary == nil || result.Summary.CoveredFrom != -4 || result.Summary.CoveredThrough != 1 || len(summarizer.received) != 3 || !strings.Contains(summarizer.received[0].Content(), "prior") {
		t.Fatalf("summary=%#v input=%#v", result.Summary, summarizer.received)
	}
}

func TestMemoryCompactorLeavesConversationBelowThresholdUnchanged(t *testing.T) {
	messages := []llm.LlmMessage{llm.NewLlmMessage("user", "u1", nil, ""), llm.NewLlmMessage("assistant", "a1", nil, "")}
	result, err := (MemoryCompactor{Summarizer: &recordingSummarizer{}}).Compact(context.Background(), CompactionInput{Messages: messages, RawTurnLimit: 2})
	if err != nil || result.Summary != nil || len(result.Messages) != 2 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestMemoryCompactorRetainsExistingSummaryBelowRawThreshold(t *testing.T) {
	messages := []llm.LlmMessage{llm.NewLlmMessage("user", "u3", nil, ""), llm.NewLlmMessage("assistant", "a3", nil, "")}
	prior := &MemorySummary{Content: "prior summary", CoveredFrom: 0, CoveredThrough: 3, Fingerprint: "old"}
	result, err := (MemoryCompactor{Summarizer: &recordingSummarizer{}}).Compact(context.Background(), CompactionInput{Messages: messages, RawTurnLimit: 2, ExistingSummary: prior})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != prior || len(result.Messages) != 3 || !strings.Contains(result.Messages[0].Content(), "prior summary") || result.Messages[1].Content() != "u3" {
		t.Fatalf("result=%#v", result)
	}
}
