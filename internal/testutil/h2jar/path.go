// Package h2jar resolves the pinned H2 test prerequisite without host-specific paths.
package h2jar

import (
	"os"
	"path/filepath"
)

func Path() (string, error) {
	if jar := os.Getenv("H2_JAR"); jar != "" {
		return jar, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".m2", "repository", "com", "h2database", "h2", "2.3.232", "h2-2.3.232.jar"), nil
}
