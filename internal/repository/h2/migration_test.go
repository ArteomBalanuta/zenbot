package h2

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"zenbot/internal/testutil/h2jar"

	_ "modernc.org/sqlite"
)

func TestOpenMigratesLegacySQLiteRowsAndArchivesSource(t *testing.T) {
	dir := t.TempDir()
	stem := filepath.Join(dir, "database")
	legacy := stem + ".db"
	createLegacySQLite(t, legacy, "legacy", 7, 1784648927381)

	database, err := Open(context.Background(), testH2Config(t, dir, stem))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var id, created int64
	var message, visibility string
	if err := database.DB.QueryRow(`SELECT id,message,created_on,visibility FROM messages WHERE name='legacy'`).Scan(&id, &message, &created, &visibility); err != nil {
		t.Fatal(err)
	}
	if id != 7 || message != "migrated" || created != 1784648927381 || visibility != "PUBLIC" {
		t.Fatalf("migrated row id=%d message=%q created=%d visibility=%q", id, message, created, visibility)
	}
	if _, err := database.DB.Exec(`INSERT INTO messages(name,message,created_on,visibility) VALUES('next','next',1,'PUBLIC')`); err != nil {
		t.Fatal(err)
	}
	var nextID int64
	if err := database.DB.QueryRow(`SELECT id FROM messages WHERE name='next'`).Scan(&nextID); err != nil || nextID <= 7 {
		t.Fatalf("next id=%d err=%v", nextID, err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy source still exists: %v", err)
	}
	if _, err := os.Stat(legacy + ".bak"); err != nil {
		t.Fatalf("legacy archive missing: %v", err)
	}
}

func TestOpenConnectsToSaturnCredentiallessH2Database(t *testing.T) {
	dir := t.TempDir()
	stem := filepath.Join(dir, "database")
	config := testH2Config(t, dir, stem)
	createSaturnH2Database(t, config.H2Jar, stem)

	database, err := Open(context.Background(), config)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var count int
	if err := database.DB.QueryRow("SELECT COUNT(*) FROM saturn_seed").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("saturn seed count = %d, want 1", count)
	}
}

func TestOpenNormalizesRelativeBaseDirectoryForPGServer(t *testing.T) {
	baseDirectory, err := os.MkdirTemp(".", "h2-relative-base-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(baseDirectory) })
	relativeBaseDirectory, err := filepath.Rel(".", baseDirectory)
	if err != nil {
		t.Fatal(err)
	}

	config := testH2Config(t, relativeBaseDirectory, "database")
	database, err := Open(context.Background(), config)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var version string
	if err := database.DB.QueryRow("SELECT H2VERSION()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version == "" {
		t.Fatal("H2 version is empty")
	}
}

func TestOpenRefusesToMergeConflictingSQLiteAndH2Data(t *testing.T) {
	dir := t.TempDir()
	stem := filepath.Join(dir, "database")
	first, err := Open(context.Background(), testH2Config(t, dir, stem))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.DB.Exec(`INSERT INTO messages(name,message,created_on,visibility) VALUES('h2','existing',1,'PUBLIC')`); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	createLegacySQLite(t, stem+".db", "sqlite", 1, 2)

	second, err := Open(context.Background(), testH2Config(t, dir, stem))
	if second != nil {
		_ = second.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "conflicting data") {
		t.Fatalf("Open() error=%v, want conflicting data refusal", err)
	}
	if _, statErr := os.Stat(stem + ".db"); statErr != nil {
		t.Fatalf("conflicting source was moved or removed: %v", statErr)
	}
}

func createLegacySQLite(t *testing.T, path, name string, id, created int64) {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`CREATE TABLE messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		trip TEXT,
		name TEXT NOT NULL,
		hash TEXT,
		message TEXT,
		created_on INTEGER NOT NULL,
		channel TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO messages(id,trip,name,hash,message,created_on,channel) VALUES(?,?,?,?,?,?,?)`, id, "trip", name, "hash", "migrated", created, "programming"); err != nil {
		t.Fatal(err)
	}
}

func createSaturnH2Database(t *testing.T, jar, stem string) {
	t.Helper()
	url := "jdbc:h2:file:" + stem + ";AUTO_SERVER=TRUE;DB_CLOSE_DELAY=-1;DATABASE_TO_LOWER=TRUE;NON_KEYWORDS=VALUE"
	command := exec.Command(
		"java", "-cp", jar, "org.h2.tools.Shell",
		"-url", url,
		"-user", "",
		"-password", "",
		"-sql", "CREATE TABLE saturn_seed(id BIGINT PRIMARY KEY); INSERT INTO saturn_seed VALUES(1)",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create Saturn H2 database: %v\n%s", err, output)
	}
}

func testH2Config(t *testing.T, dir, stem string) Config {
	t.Helper()
	jar, err := h2jar.Path()
	if err != nil {
		t.Fatal(err)
	}
	return Config{BaseDir: dir, DatabaseStem: stem, H2Jar: jar, Host: "127.0.0.1", AutoPort: true, StartupTimeout: 5 * time.Second}
}
