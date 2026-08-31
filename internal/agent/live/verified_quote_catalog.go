package live

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const verifiedQuoteResource = "verified-quotes.json"

type verifiedQuote struct {
	ID        string `json:"id"`
	Quote     string `json:"quote"`
	Book      string `json:"book"`
	Author    string `json:"author"`
	Reference string `json:"reference"`
}

type verifiedQuoteCatalog struct{ entries []verifiedQuote }

type verifiedQuoteResourceSource func(string) (string, error)

func loadVerifiedQuoteCatalog(source verifiedQuoteResourceSource) (verifiedQuoteCatalog, error) {
	if source == nil {
		source = verifiedQuoteFilesystemResource
	}
	text, err := source(verifiedQuoteResource)
	if err != nil {
		return verifiedQuoteCatalog{}, fmt.Errorf("cannot load verified quote resource: %w", err)
	}
	var entries []verifiedQuote
	if err := json.Unmarshal([]byte(text), &entries); err != nil {
		return verifiedQuoteCatalog{}, fmt.Errorf("cannot decode verified quote resource: %w", err)
	}
	if len(entries) == 0 {
		return verifiedQuoteCatalog{}, errors.New("verified quote catalog must not be empty")
	}
	ids, lines := map[string]bool{}, map[string]bool{}
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.Quote) == "" || strings.TrimSpace(entry.Book) == "" || strings.TrimSpace(entry.Author) == "" || strings.TrimSpace(entry.Reference) == "" {
			return verifiedQuoteCatalog{}, errors.New("verified quote catalog has blank required field")
		}
		if containsControlNewline(entry.ID) || containsControlNewline(entry.Quote) || containsControlNewline(entry.Book) || containsControlNewline(entry.Author) || containsControlNewline(entry.Reference) {
			return verifiedQuoteCatalog{}, errors.New("verified quote catalog has control newline")
		}
		line := entry.line()
		if ids[entry.ID] || lines[line] {
			return verifiedQuoteCatalog{}, errors.New("verified quote catalog has duplicate entry")
		}
		if !validVerifiedQuoteLine(line) {
			return verifiedQuoteCatalog{}, errors.New("verified quote catalog has invalid quote syntax")
		}
		ids[entry.ID], lines[line] = true, true
	}
	return verifiedQuoteCatalog{entries: append([]verifiedQuote(nil), entries...)}, nil
}

func (e verifiedQuote) line() string            { return `"` + e.Quote + `" — ` + e.Book + `, ` + e.Author }
func (c verifiedQuoteCatalog) fallback() string { return c.entries[0].line() }
func (c verifiedQuoteCatalog) find(content string) (string, bool) {
	content = stripJavaWhitespace(content)
	for _, entry := range c.entries {
		if content == entry.line() {
			return entry.line(), true
		}
	}
	return "", false
}
func (c verifiedQuoteCatalog) selectVerifiedOrFallback(content string) string {
	if line, ok := c.find(content); ok {
		return line
	}
	return c.fallback()
}
func containsControlNewline(value string) bool { return strings.ContainsAny(value, "\r\n") }
func validVerifiedQuoteLine(line string) bool {
	if strings.ContainsAny(line, "\r\n") || !strings.HasPrefix(line, `"`) || strings.HasPrefix(line, `-`) {
		return false
	}
	quoteEnd := strings.Index(line[1:], `"`)
	if quoteEnd < 0 {
		return false
	}
	quoteEnd++
	return quoteEnd > 1 && strings.HasPrefix(line[quoteEnd+1:], " — ") && strings.Contains(line[quoteEnd+4:], ", ")
}
func verifiedQuoteFilesystemResource(name string) (string, error) {
	candidates := []string{filepath.Join("resources", "agent", name)}
	if _, source, _, ok := runtime.Caller(0); ok {
		root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", ".."))
		candidates = append(candidates, filepath.Join(root, "resources", "agent", name))
	}
	for _, candidate := range candidates {
		contents, err := os.ReadFile(candidate)
		if err == nil {
			return string(contents), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
	}
	return "", errors.New("verified quote resource is missing")
}
