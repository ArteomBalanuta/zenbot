// Package h2fixture provides per-test real-H2 fixtures. It is test support only.
package h2fixture

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zenbot/internal/repository/h2"
)

const pinnedH2Jar = "/Users/ab/.m2/repository/com/h2database/h2/2.3.232/h2-2.3.232.jar"

// Open creates one independently owned H2 PostgreSQL-wire fixture for t.
func Open(t testing.TB, stem string) *h2.Database {
	t.Helper()
	if stem == "" || stem == "." || strings.ContainsAny(stem, `/\\`) || strings.HasSuffix(stem, ".db") || strings.HasSuffix(stem, ".mv.db") {
		t.Fatalf("invalid H2 fixture stem %q", stem)
	}
	jar := os.Getenv("H2_JAR")
	if jar == "" {
		jar = pinnedH2Jar
	}
	if _, err := os.Stat(jar); err != nil {
		t.Fatalf("H2 fixture jar: %v", err)
	}
	dir := t.TempDir()
	d, err := h2.Open(context.Background(), h2.Config{
		BaseDir:        dir,
		DatabaseStem:   filepath.Join(dir, stem),
		H2Jar:          jar,
		Host:           "127.0.0.1",
		AutoPort:       true,
		StartupTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Errorf("close H2 fixture: %v", err)
		}
	})
	return d
}
