package h2

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRealH2PostgresWire(t *testing.T) {
	jar := os.Getenv("H2_JAR")
	if jar == "" {
		jar = "/Users/ab/.m2/repository/com/h2database/h2/2.3.232/h2-2.3.232.jar"
	}
	if _, err := os.Stat(jar); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	d, err := Open(context.Background(), Config{BaseDir: dir, DatabaseStem: filepath.Join(dir, "zenbot.db"), H2Jar: jar, Port: 55435, StartupTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var version string
	if err = d.DB.QueryRow("SELECT H2VERSION()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version == "" {
		t.Fatal("empty H2 version")
	}
	if _, err = d.DB.Exec("INSERT INTO messages(trip,name,message,created_on,visibility) VALUES($1,$2,$3,$4,$5)", "t", "n", "m", int64(1), "PUBLIC"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = d.DB.QueryRow("SELECT COUNT(*) FROM messages").Scan(&n); err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}
}
