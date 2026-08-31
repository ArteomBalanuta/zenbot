package live

import (
	"testing"

	"zenbot/internal/agent/participation"
	"zenbot/internal/agent/runtime"
)

func TestVerifiedQuoteCatalogLoadsResourceAndCanonicalizes(t *testing.T) {
	catalog, err := loadVerifiedQuoteCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.entries) != 3 {
		t.Fatalf("entries = %d", len(catalog.entries))
	}
	first := `"It is a truth universally acknowledged, that a single man in possession of a good fortune, must be in want of a wife." — Pride and Prejudice, Jane Austen`
	if got := catalog.fallback(); got != first {
		t.Fatalf("fallback = %q", got)
	}
	if got, ok := catalog.find("\t" + first + "\n"); !ok || got != first {
		t.Fatalf("canonical lookup = %q, %v", got, ok)
	}
}

func TestVerifiedQuoteCatalogRejectsInvalidEntries(t *testing.T) {
	for name, resource := range map[string]string{
		"empty":          `[]`,
		"blank field":    `[{"id":"id","quote":"quote","book":"book","author":"","reference":"ref"}]`,
		"duplicate id":   `[{"id":"id","quote":"one","book":"book","author":"author","reference":"ref"},{"id":"id","quote":"two","book":"book","author":"author","reference":"ref"}]`,
		"duplicate line": `[{"id":"one","quote":"quote","book":"book","author":"author","reference":"ref"},{"id":"two","quote":"quote","book":"book","author":"author","reference":"ref"}]`,
		"newline":        `[{"id":"id","quote":"one\ntwo","book":"book","author":"author","reference":"ref"}]`,
		"legacy dash":    `[{"id":"id","quote":"\"quote\"","book":"book","author":"author","reference":"ref"}]`,
		"malformed":      `{`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadVerifiedQuoteCatalog(func(string) (string, error) { return resource, nil })
			if err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
}

func TestOutputFinalizerUsesVerifiedQuoteForEligiblePublicResponse(t *testing.T) {
	catalog, err := loadVerifiedQuoteCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	f := OutputFinalizer{NoReplyMarker: "[[SATURN_NO_REPLY]]", Catalog: &catalog}
	inv := testInvocation("hello?", false)
	if got, reply, err := f.FinalizeWithContext(inv, "ordinary prose", FinalizationContext{CandidateKind: participation.Talk}); err != nil || !reply || got != catalog.fallback() {
		t.Fatalf("fallback = %q, %v, %v", got, reply, err)
	}
	line := catalog.fallback()
	if got, reply, err := f.FinalizeWithContext(inv, "	"+line+"\n", FinalizationContext{CandidateKind: participation.Talk}); err != nil || !reply || got != line {
		t.Fatalf("canonical = %q, %v, %v", got, reply, err)
	}
	if got, reply, err := f.FinalizeWithContext(inv, "ordinary prose", FinalizationContext{CandidateKind: participation.Talk, ToolAttempted: true}); err != nil || !reply || got != "ordinary prose" {
		t.Fatalf("attempt exemption = %q, %v, %v", got, reply, err)
	}
	if _, reply, err := f.FinalizeWithContext(runtime.NewInvocation("ambient", runtime.NewContext("r", "n", "", "", false, nil), "hello?", runtime.AMBIENT, "", false), "[[SATURN_NO_REPLY]]", FinalizationContext{CandidateKind: participation.Talk}); err != nil || reply {
		t.Fatalf("marker behavior = %v, %v", reply, err)
	}
}

func TestOutputFinalizerDoesNotRequireQuotesForWhispersOrCommandsOrModeration(t *testing.T) {
	catalog, err := loadVerifiedQuoteCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	f := OutputFinalizer{Catalog: &catalog}
	for name, inv := range map[string]runtime.Invocation{
		"whisper":    testWhisperInvocation(),
		"command":    runtime.NewInvocation("direct", runtime.NewContext("r", "n", "", "", false, nil), "hello?", runtime.DIRECT, "", true),
		"moderation": runtime.NewInvocation("mod", runtime.NewContext("r", "n", "", "", false, nil), "hello?", runtime.MODERATION, "", false),
	} {
		t.Run(name, func(t *testing.T) {
			got, reply, err := f.FinalizeWithContext(inv, "ordinary prose", FinalizationContext{CandidateKind: participation.Talk})
			if err != nil || !reply || got != "ordinary prose" {
				t.Fatalf("result = %q, %v, %v", got, reply, err)
			}
		})
	}
}

func TestOutputFinalizerFailsClosedForEligibleResponseWithoutCatalog(t *testing.T) {
	f := OutputFinalizer{NoReplyMarker: "[[SATURN_NO_REPLY]]"}
	inv := testInvocation("hello?", false)
	if _, _, err := f.FinalizeWithContext(inv, "provider prose", FinalizationContext{CandidateKind: participation.Talk}); err == nil {
		t.Fatal("eligible response without a verified catalog was delivered")
	}
}

func TestVerifiedQuoteCatalogRejectsControlNewlinesInEveryField(t *testing.T) {
	for name, resource := range map[string]string{
		"id carriage return": `[{"id":"id\r","quote":"quote","book":"book","author":"author","reference":"ref"}]`,
		"reference newline":  `[{"id":"id","quote":"quote","book":"book","author":"author","reference":"ref\n"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadVerifiedQuoteCatalog(func(string) (string, error) { return resource, nil }); err == nil {
				t.Fatal("catalog accepted control newline")
			}
		})
	}
}

func testInvocation(prompt string, command bool) runtime.Invocation {
	return runtime.NewInvocation("id", runtime.NewContext("r", "n", "", "", false, nil), prompt, runtime.MENTION, "", command)
}
func testWhisperInvocation() runtime.Invocation {
	return runtime.NewInvocation("whisper", runtime.NewContext("r", "n", "", "", true, nil), "hello?", runtime.MENTION, "", false)
}
