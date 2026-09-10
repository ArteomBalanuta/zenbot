package turn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"zenbot/internal/agent/llm"
)

type ConversationSummarizer interface {
	Summarize(context.Context, []llm.LlmMessage) (string, error)
}

type MemorySummary struct {
	Content        string
	CoveredFrom    int64
	CoveredThrough int64
	Fingerprint    string
	SourceCursor   int64
}

type CompactionInput struct {
	Messages        []llm.LlmMessage
	RawTurnLimit    int
	ExistingSummary *MemorySummary
}

type CompactionResult struct {
	Messages []llm.LlmMessage
	Summary  *MemorySummary
}

type MemoryCompactor struct{ Summarizer ConversationSummarizer }

func (c MemoryCompactor) Compact(ctx context.Context, input CompactionInput) (CompactionResult, error) {
	if err := ctx.Err(); err != nil {
		return CompactionResult{}, err
	}
	if input.RawTurnLimit < 1 {
		return CompactionResult{}, fmt.Errorf("raw turn limit must be positive")
	}
	raw := copyConversationMessages(input.Messages)
	completeTurns := countLeadingCompleteTurns(raw)
	if completeTurns <= input.RawTurnLimit {
		if input.ExistingSummary == nil {
			return CompactionResult{Messages: raw}, nil
		}
		projected := append([]llm.LlmMessage{summaryMessage(*input.ExistingSummary)}, raw...)
		return CompactionResult{Messages: projected, Summary: input.ExistingSummary}, nil
	}
	if c.Summarizer == nil {
		return CompactionResult{}, fmt.Errorf("conversation summarizer is required")
	}
	compactTurns := completeTurns - input.RawTurnLimit
	compactMessages := copyConversationMessages(raw[:compactTurns*2])
	coveredFrom := int64(0)
	coveredThrough := int64(compactTurns*2 - 1)
	if input.ExistingSummary != nil {
		coveredFrom = input.ExistingSummary.CoveredFrom
		coveredThrough = input.ExistingSummary.CoveredThrough + int64(compactTurns*2)
		compactMessages = append([]llm.LlmMessage{summaryMessage(*input.ExistingSummary)}, compactMessages...)
	}
	content, err := c.Summarizer.Summarize(ctx, compactMessages)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("summarize conversation: %w", err)
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return CompactionResult{}, fmt.Errorf("conversation summary is empty")
	}
	summary := &MemorySummary{
		Content:        content,
		CoveredFrom:    coveredFrom,
		CoveredThrough: coveredThrough,
		Fingerprint:    conversationFingerprint(compactMessages),
	}
	projected := []llm.LlmMessage{summaryMessage(*summary)}
	projected = append(projected, copyConversationMessages(raw[compactTurns*2:])...)
	return CompactionResult{Messages: projected, Summary: summary}, nil
}

func countLeadingCompleteTurns(messages []llm.LlmMessage) int {
	turns := 0
	for index := 0; index+1 < len(messages); index += 2 {
		if messages[index].Role() != "user" || messages[index+1].Role() != "assistant" {
			break
		}
		turns++
	}
	return turns
}

func summaryMessage(summary MemorySummary) llm.LlmMessage {
	content := fmt.Sprintf("UNTRUSTED_CONVERSATION_SUMMARY covered=%d..%d fingerprint=%s\n%s", summary.CoveredFrom, summary.CoveredThrough, summary.Fingerprint, summary.Content)
	return llm.NewLlmMessage("user", content, nil, "")
}

func copyConversationMessages(messages []llm.LlmMessage) []llm.LlmMessage {
	out := make([]llm.LlmMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, llm.NewLlmMessage(message.Role(), message.Content(), message.ToolCalls(), message.ToolCallID()))
	}
	return out
}

func conversationFingerprint(messages []llm.LlmMessage) string {
	hash := sha256.New()
	for _, message := range messages {
		hash.Write([]byte(message.Role()))
		hash.Write([]byte{0})
		hash.Write([]byte(message.Content()))
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
