package live

import "testing"

func TestResponseSanitizerRemovesLegacyPersonaAndFormatsLists(t *testing.T) {
	sanitizer := responseSanitizer{}
	got := sanitizer.sanitize("[sips tea]\nAh, mer.\n* weather was sunny\nCarpe diem, mer.")
	if want := "\u2009-\u2009weather was sunny"; got != want {
		t.Fatalf("sanitize() = %q, want %q", got, want)
	}
	if got, want := sanitizer.sanitize("first\nThe archives reveal old prose\nsecond"), "first\nsecond"; got != want {
		t.Fatalf("boilerplate removal = %q, want %q", got, want)
	}
}

func TestResponseSanitizerPreservesOrdinaryEvidence(t *testing.T) {
	sanitizer := responseSanitizer{}
	got := sanitizer.sanitize("Relevant records reveal useful database evidence.\n• plain fact")
	if want := "Relevant records reveal useful database evidence.\n\u2009-\u2009plain fact"; got != want {
		t.Fatalf("sanitize() = %q, want %q", got, want)
	}
	if sanitizer.containsLegacyPersona("Relevant records reveal useful database evidence.") {
		t.Fatal("ordinary evidence identified as legacy persona")
	}
	if !sanitizer.containsLegacyPersona("[sips tea]") {
		t.Fatal("legacy marker not identified")
	}
}

func TestResponseSanitizerNormalizesBlankAndNumberedLists(t *testing.T) {
	sanitizer := responseSanitizer{}
	if got := sanitizer.sanitize(" \n\t"); got != "" {
		t.Fatalf("blank sanitize() = %q", got)
	}
	if got, want := sanitizer.sanitize("1. first\n2) second"), "\u2009-\u2009first\n\u2009-\u2009second"; got != want {
		t.Fatalf("sanitize() = %q, want %q", got, want)
	}
}

func TestResponseSanitizerPreservesUnicodeContent(t *testing.T) {
	sanitizer := responseSanitizer{}
	if got, want := sanitizer.sanitize("Привет 😀\n* 東京"), "Привет 😀\n\u2009-\u2009東京"; got != want {
		t.Fatalf("sanitize() = %q, want %q", got, want)
	}
}

func TestResponseSanitizerPreservesNonBreakingSpaceLikeSaturn(t *testing.T) {
	sanitizer := responseSanitizer{}
	if got, want := sanitizer.sanitize("ordinary\u00a0"), "ordinary\u00a0"; got != want {
		t.Fatalf("sanitize() = %q, want %q", got, want)
	}
}
