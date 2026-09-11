package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
)

func TestSQLiteRuntimeReopenAndConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "database ?#.db")
	d, err := Open(context.Background(), Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := d.DB.Exec(`INSERT INTO messages(name,message,created_on) VALUES('n','m',1784648927381)`); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(context.Background(), Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var count int
	if err := d.DB.QueryRow(`SELECT COUNT(*) FROM messages WHERE created_on=1784648927381`).Scan(&count); err != nil || count != 20 {
		t.Fatalf("count=%d error=%v", count, err)
	}
	var mode string
	if err := d.DB.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("mode=%s error=%v", mode, err)
	}
}

func TestSQLiteForeignKeysOnEveryConnection(t *testing.T) {
	d, err := Open(context.Background(), Config{Path: filepath.Join(t.TempDir(), "database.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var conns []*sql.Conn
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	for i := 0; i < 4; i++ {
		c, err := d.DB.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
		if _, err := c.ExecContext(context.Background(), `INSERT INTO trip_names(trip_id,name_id) VALUES(999,999)`); err == nil {
			t.Fatal("orphan accepted")
		}
	}
}

func TestSQLiteOpenRejectsEmptyPathAndCancelledContext(t *testing.T) {
	if d, err := Open(context.Background(), Config{}); err == nil {
		d.Close()
		t.Fatal("empty path accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if d, err := Open(ctx, Config{Path: filepath.Join(t.TempDir(), "database.db")}); err == nil {
		d.Close()
		t.Fatal("cancelled open accepted")
	}
}

func TestSQLiteConcurrentOpenAndTransactionRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")
	d, err := Open(context.Background(), Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other, err := Open(context.Background(), Config{Path: path})
			if err != nil {
				t.Error(err)
				return
			}
			defer other.Close()
			tx, err := other.DB.BeginTx(context.Background(), nil)
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := tx.Exec(`INSERT INTO messages(name,message,created_on) VALUES('rollback','row',1)`); err != nil {
				t.Error(err)
			}
			if err := tx.Rollback(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var count int
	if err := d.DB.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rolled-back rows=%d error=%v", count, err)
	}
}
