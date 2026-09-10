package assemble

import (
	"encoding/json"
	"strings"
	"testing"

	"zenbot/internal/agent/llm"
)

func TestContextBudgeterKeepsCompleteUnitsAndValidJSON(t *testing.T) {
	recent := `{"rows":[{"name":"alice","message":"` + strings.Repeat("x", 400) + `"}]}`
	input := ContextInput{
		RequiredPrefix: []Message{llm.NewLlmMessage("system", "policy", nil, "")},
		Optional: []ContextUnit{{
			Source:    ContextRecentRoom,
			Timestamp: 20,
			Priority:  100,
			Messages:  []Message{llm.NewLlmMessage("user", recent, nil, "")},
		}},
		RequiredSuffix: []Message{llm.NewLlmMessage("user", "newest request", nil, "")},
		MaxTokens:      20,
	}
	projection, err := (ContextBudgeter{}).Project(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Messages) != 2 || projection.Messages[1].Content() != "newest request" {
		t.Fatalf("projection split or retained oversized unit: %#v", projection.Messages)
	}
	if !json.Valid([]byte(recent)) {
		t.Fatal("test fixture is not valid JSON")
	}
}

func TestContextBudgeterDeduplicatesAcrossSourcesAndKeepsHigherPriorityUnit(t *testing.T) {
	duplicate := llm.NewLlmMessage("user", "same evidence", nil, "")
	projection, err := (ContextBudgeter{}).Project(ContextInput{
		RequiredPrefix: []Message{llm.NewLlmMessage("system", "policy", nil, "")},
		Optional: []ContextUnit{
			{Source: ContextMemory, Timestamp: 10, Priority: 10, Messages: []Message{duplicate}},
			{Source: ContextRecentRoom, Timestamp: 20, Priority: 20, Messages: []Message{duplicate}},
			{Source: ContextRecentRoom, Timestamp: 30, Priority: 20, Messages: []Message{llm.NewLlmMessage("user", "newer", nil, "")}},
		},
		RequiredSuffix: []Message{llm.NewLlmMessage("user", "current", nil, "")},
		MaxTokens:      100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if projection.RemovedUnits != 1 || len(projection.Messages) != 4 {
		t.Fatalf("deduplicated projection=%#v", projection)
	}
	if projection.Messages[1].Content() != "same evidence" || projection.Messages[2].Content() != "newer" {
		t.Fatalf("source order was not deterministic: %#v", projection.Messages)
	}
}

func TestContextBudgeterSupportsOneMillionTokenCeiling(t *testing.T) {
	projection, err := (ContextBudgeter{}).Project(ContextInput{
		RequiredPrefix: []Message{llm.NewLlmMessage("system", "policy", nil, "")},
		RequiredSuffix: []Message{llm.NewLlmMessage("user", "current", nil, "")},
		MaxTokens:      1_000_000,
		ReserveTokens:  100_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if projection.BudgetChars != 3_600_000 {
		t.Fatalf("budget chars=%d, want 3600000", projection.BudgetChars)
	}
	if _, err := (ContextBudgeter{}).Project(ContextInput{MaxTokens: 1_000_001}); err == nil {
		t.Fatal("context ceiling above one million tokens was accepted")
	}
}

func TestContextBudgeterReservesProviderToolManifestCapacity(t *testing.T) {
	projection, err := (ContextBudgeter{}).Project(ContextInput{
		RequiredPrefix: []Message{llm.NewLlmMessage("system", "policy", nil, "")},
		RequiredSuffix: []Message{llm.NewLlmMessage("user", "current", nil, "")},
		MaxTokens:      100,
		ReserveTokens:  10,
		ManifestTokens: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if projection.BudgetChars != 240 {
		t.Fatalf("budget chars=%d, want 240", projection.BudgetChars)
	}
	if _, err := (ContextBudgeter{}).Project(ContextInput{MaxTokens: 20, ReserveTokens: 10, ManifestTokens: 10}); err == nil {
		t.Fatal("manifest exhausted context capacity without an error")
	}
}
