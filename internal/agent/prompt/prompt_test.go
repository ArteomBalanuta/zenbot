package prompt

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestDefaultCatalogLoadsRepositoryAgentResources(t *testing.T) {
	catalog, err := NewCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := catalog.ToolDescription("run_command"); err != nil || !strings.Contains(got, "approved Saturn command") {
		t.Fatalf("default ToolDescription() = %q, %v", got, err)
	}
}

func TestCatalogTextPreservesUTF8WhitespaceAndTrailingNewline(t *testing.T) {
	catalog := newTestCatalog(t, map[string]string{
		"tool-copy.json": "{}",
		"utf8.txt":       "  Привет, 世界 🌍  \n\n",
	})

	got, err := catalog.Text("utf8.txt")
	if err != nil {
		t.Fatal(err)
	}
	if want := "  Привет, 世界 🌍  \n\n"; got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
}

func TestCatalogFormattedStripsOnlyTrailingWhitespaceBeforeFormatting(t *testing.T) {
	catalog := newTestCatalog(t, map[string]string{
		"tool-copy.json": "{}",
		"format.txt":     "  hello %s  \n\t\n",
	})

	got, err := catalog.Formatted("format.txt", "世界")
	if err != nil {
		t.Fatal(err)
	}
	if want := "  hello 世界"; got != want {
		t.Fatalf("Formatted() = %q, want %q", got, want)
	}
}

func TestCatalogReportsMissingAndIOResourceErrors(t *testing.T) {
	catalog := newTestCatalog(t, map[string]string{"tool-copy.json": "{}"})

	_, err := catalog.Text("missing.txt")
	if err == nil || err.Error() != "Missing agent prompt resource: /agent/missing.txt" {
		t.Fatalf("missing error = %v", err)
	}

	cause := errors.New("read failed")
	catalog, err = NewCatalog(func(name string) (io.ReadCloser, error) {
		if name == "tool-copy.json" {
			return io.NopCloser(strings.NewReader("{}")), nil
		}
		return nil, cause
	})
	_, err = catalog.Text("broken.txt")
	if err == nil || err.Error() != "Cannot load agent prompt resource: /agent/broken.txt: read failed" {
		t.Fatalf("I/O error = %v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("I/O error does not wrap cause: %v", err)
	}
}

func TestCatalogLoadsToolDescriptionGuidanceAndExample(t *testing.T) {
	catalog := newTestCatalog(t, map[string]string{
		"tool-copy.json": `{"weather":{"description":"Fetch weather","whenToUse":["for forecasts","for current conditions"],"example":"{\"city\":\"Tokyo\"} - Fetch Tokyo weather"}}`,
	})

	if got, err := catalog.ToolDescription("weather"); err != nil || got != "Fetch weather" {
		t.Fatalf("ToolDescription() = %q, %v", got, err)
	}
	guidance, err := catalog.ToolGuidance("weather", "whenToUse")
	if err != nil || fmt.Sprint(guidance) != "[for forecasts for current conditions]" {
		t.Fatalf("ToolGuidance() = %v, %v", guidance, err)
	}
	if got, err := catalog.ToolExample("weather"); err != nil || got != `{"city":"Tokyo"} - Fetch Tokyo weather` {
		t.Fatalf("ToolExample() = %q, %v", got, err)
	}
}

func TestCatalogRejectsUnknownAndNonObjectToolEntries(t *testing.T) {
	catalog := newTestCatalog(t, map[string]string{
		"tool-copy.json": `{"broken":"not-an-object"}`,
	})
	for _, name := range []string{"unknown", "broken"} {
		_, err := catalog.ToolDescription(name)
		if err == nil || err.Error() != "Missing agent tool copy: "+name {
			t.Fatalf("ToolDescription(%q) error = %v", name, err)
		}
	}
}

func TestCatalogRejectsMalformedToolCopyDuringConstruction(t *testing.T) {
	_, err := NewCatalog(func(name string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("{")), nil
	})
	if err == nil {
		t.Fatal("NewCatalog() unexpectedly succeeded")
	}
	if err == nil || !strings.HasPrefix(err.Error(), "Cannot load agent prompt resource: /agent/tool-copy.json:") {
		t.Fatalf("malformed tool copy error = %v", err)
	}
}

func newTestCatalog(t *testing.T, resources map[string]string) *Catalog {
	t.Helper()
	catalog, err := NewCatalog(func(name string) (io.ReadCloser, error) {
		value, ok := resources[name]
		if !ok {
			return nil, nil
		}
		return io.NopCloser(strings.NewReader(value)), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
