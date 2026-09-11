package h2jar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathUsesExplicitOverride(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom.jar")
	t.Setenv("H2_JAR", want)
	got, err := Path()
	if err != nil || got != want {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestPathUsesCurrentUsersMavenCache(t *testing.T) {
	t.Setenv("H2_JAR", "")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(homeDir, ".m2", "repository", "com", "h2database", "h2", "2.3.232", "h2-2.3.232.jar")
	got, err := Path()
	if err != nil || got != want {
		t.Fatalf("got=%q want=%q err=%v", got, want, err)
	}
}
