// Package sqlitefixture provides isolated on-disk SQLite fixtures.
package sqlitefixture

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"zenbot/internal/repository/sqlite"
)

func Open(t testing.TB, stem string) *sqlite.Database {
	t.Helper()
	if stem == "" || stem == "." || stem == ".." || strings.ContainsAny(stem, `/\`) {
		t.Fatalf("invalid SQLite fixture stem %q", stem)
	}
	d, err := sqlite.Open(context.Background(), sqlite.Config{Path: filepath.Join(t.TempDir(), stem+".db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Errorf("close SQLite fixture: %v", err)
		}
	})
	return d
}
