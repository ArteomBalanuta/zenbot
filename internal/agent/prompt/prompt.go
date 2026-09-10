// Package prompt provides the unwired agent prompt catalog.
package prompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

const resourceRoot = "/agent/"

type resourceSource func(name string) (io.ReadCloser, error)

type Catalog struct {
	resources resourceSource

	toolCopy map[string]json.RawMessage
}

// NewCatalog creates a prompt catalog and loads its tool-copy resource.
func NewCatalog(resources resourceSource) (*Catalog, error) {
	if resources == nil {
		resources = filesystemResource
	}
	catalog := &Catalog{resources: resources}
	reader, err := catalog.open("tool-copy.json")
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	if err := json.NewDecoder(reader).Decode(&catalog.toolCopy); err != nil {
		return nil, catalog.loadFailure("tool-copy.json", err)
	}
	if catalog.toolCopy == nil {
		return nil, catalog.loadFailure("tool-copy.json", errors.New("Agent tool copy must be a JSON object"))
	}
	return catalog, nil
}

// Text returns a resource byte-for-byte decoded as UTF-8, including whitespace and newlines.
func (c *Catalog) Text(name string) (string, error) {
	reader, err := c.open(name)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	contents, err := io.ReadAll(reader)
	if err != nil {
		return "", c.loadFailure(name, err)
	}
	return string(contents), nil
}

// Formatted strips trailing Unicode whitespace before applying fmt formatting.
func (c *Catalog) Formatted(name string, args ...any) (string, error) {
	text, err := c.Text(name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(strings.TrimRightFunc(text, unicode.IsSpace), args...), nil
}

// ToolDescription returns the description for a named tool.
func (c *Catalog) ToolDescription(name string) (string, error) {
	entry, err := c.tool(name)
	if err != nil {
		return "", err
	}
	var value string
	if err := json.Unmarshal(entry["description"], &value); err != nil {
		return "", err
	}
	return value, nil
}

// ToolGuidance returns a string-list guidance field for a named tool.
func (c *Catalog) ToolGuidance(name, field string) ([]string, error) {
	entry, err := c.tool(name)
	if err != nil {
		return nil, err
	}
	var values []string
	if err := json.Unmarshal(entry[field], &values); err != nil {
		return nil, err
	}
	return values, nil
}

// ToolExample returns the example for a named tool.
func (c *Catalog) ToolExample(name string) (string, error) {
	entry, err := c.tool(name)
	if err != nil {
		return "", err
	}
	var value string
	if err := json.Unmarshal(entry["example"], &value); err != nil {
		return "", err
	}
	return value, nil
}

func (c *Catalog) tool(name string) (map[string]json.RawMessage, error) {
	raw, ok := c.toolCopy[name]
	if !ok {
		return nil, fmt.Errorf("Missing agent tool copy: %s", name)
	}
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entry); err != nil || entry == nil {
		return nil, fmt.Errorf("Missing agent tool copy: %s", name)
	}
	return entry, nil
}

func (c *Catalog) open(name string) (io.ReadCloser, error) {
	reader, err := c.resources(name)
	if err != nil {
		return nil, c.loadFailure(name, err)
	}
	if reader == nil {
		return nil, fmt.Errorf("Missing agent prompt resource: %s%s", resourceRoot, name)
	}
	return reader, nil
}

func (c *Catalog) loadFailure(name string, cause error) error {
	return fmt.Errorf("Cannot load agent prompt resource: %s%s: %w", resourceRoot, name, cause)
}

func filesystemResource(name string) (io.ReadCloser, error) {
	candidates := []string{filepath.Join("resources", "agent", name)}
	if _, source, _, ok := runtime.Caller(0); ok {
		root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", ".."))
		candidates = append(candidates, filepath.Join(root, "resources", "agent", name))
	}
	for _, candidate := range candidates {
		reader, err := os.Open(candidate)
		if err == nil {
			return reader, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return nil, nil
}
