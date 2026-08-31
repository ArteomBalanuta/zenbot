package h2

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const pinnedH2Jar = "/Users/ab/.m2/repository/com/h2database/h2/2.3.232/h2-2.3.232.jar"

func TestOpenAutoPortEmptyH2VersionClosesOwnedChild(t *testing.T) {
	jar := os.Getenv("H2_JAR")
	if jar == "" {
		jar = pinnedH2Jar
	}
	if _, err := os.Stat(jar); err != nil {
		t.Fatalf("H2 fixture jar: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	dir := t.TempDir()
	stem := filepath.Join(dir, "empty-version")

	var started *processServer
	identityRead := false
	hooks := &h2OpenTestHooks{
		onStarted: func(s *processServer) { started = s },
		h2Version: func(context.Context, *sql.DB) (string, error) {
			identityRead = true
			return "", nil
		},
	}

	db, err := Open(ctx, Config{
		BaseDir:        dir,
		DatabaseStem:   stem,
		H2Jar:          jar,
		Host:           "127.0.0.1",
		AutoPort:       true,
		StartupTimeout: 5 * time.Second,
		testHooks:      hooks,
	})
	if db != nil {
		t.Fatal("Open returned a database")
	}
	if !identityRead {
		t.Fatal("H2 identity reader was not called")
	}
	if err == nil {
		t.Fatal("Open returned nil error")
	}
	if err.Error() != "H2 identity check returned empty version" {
		t.Fatalf("Open error = %q, want %q", err, "H2 identity check returned empty version")
	}
	if started == nil {
		t.Fatal("successful H2 server was not observed")
	}
	if started.cmd == nil || started.cmd.Process == nil {
		t.Fatal("observed H2 server has no owned child process")
	}
	if started.Addr() == "" {
		t.Fatal("observed H2 server has no auto-port address")
	}
	if started.cmd.ProcessState == nil || !started.cmd.ProcessState.Exited() {
		t.Fatal("owned H2 child has not exited after Open returned")
	}
	select {
	case <-started.waitDone:
	default:
		t.Fatal("owned H2 child waitDone is not closed after Open returned")
	}
	if err := started.Stop(context.Background()); err != nil {
		t.Fatalf("idempotent H2 server stop: %v", err)
	}
}
