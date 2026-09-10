// Package release exposes the version embedded in each Zenbot build.
package release

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var version string

// Version returns the release identifier from the repository's VERSION file.
func Version() string {
	return strings.TrimSpace(version)
}
