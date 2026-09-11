package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

func seedLegacySQLite(t *testing.T, path string, invalid bool) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE messages(id INTEGER PRIMARY KEY AUTOINCREMENT,trip TEXT,name TEXT NOT NULL,hash TEXT,message TEXT,created_on INTEGER NOT NULL,channel TEXT)`,
		`INSERT INTO messages(id,name,message,created_on) VALUES(7,'legacy','a literal \n',1784648927381)`,
		`CREATE TABLE trips(id INTEGER PRIMARY KEY AUTOINCREMENT,type TEXT NOT NULL CHECK(type IN ('ADMIN','MODERATOR','TRUSTED','USER','REGULAR')),trip TEXT UNIQUE,created_on INTEGER NOT NULL)`,
		`INSERT INTO trips VALUES(3,'USER','trip',1)`,
		`INSERT INTO trips VALUES(100,'USER','deleted',1)`,
		`DELETE FROM trips WHERE id=100`,
		`CREATE TABLE names(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT UNIQUE,created_on INTEGER NOT NULL)`,
		`INSERT INTO names VALUES(4,'name',1)`,
		`CREATE TABLE trip_names(id INTEGER PRIMARY KEY AUTOINCREMENT,trip_id INTEGER NOT NULL REFERENCES trips(id),name_id INTEGER NOT NULL REFERENCES names(id),UNIQUE(trip_id,name_id))`,
		`INSERT INTO trip_names VALUES(5,3,4)`,
		`CREATE INDEX custom_trip_created ON trips(created_on)`,
		`CREATE TABLE mail(id INTEGER PRIMARY KEY AUTOINCREMENT,owner TEXT NOT NULL,receiver TEXT NOT NULL,message TEXT,status TEXT NOT NULL,created_on INTEGER NOT NULL,is_whisper TEXT)`,
		`INSERT INTO mail VALUES(8,'sender','trip','literal \n','PENDING',1,'true')`,
	}
	if invalid {
		statements = append(statements, `INSERT INTO trip_names VALUES(6,999,4)`)
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBootstrapUpgradesLegacySQLiteInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	seedLegacySQLite(t, path, false)
	for i := 0; i < 2; i++ {
		d, err := Open(context.Background(), Config{Path: path})
		if err != nil {
			t.Fatal(err)
		}
		var id, created int64
		var message, visibility string
		if err := d.DB.QueryRow(`SELECT id,message,created_on,visibility FROM messages WHERE name='legacy'`).Scan(&id, &message, &created, &visibility); err != nil {
			t.Fatal(err)
		}
		if id != 7 || created != 1784648927381 || message != `a literal \n` || visibility != "PUBLIC" {
			t.Fatalf("legacy changed: %d %d %q %q", id, created, message, visibility)
		}
		var encoding string
		if err := d.DB.QueryRow(`SELECT text_encoding FROM mail WHERE id=8`).Scan(&encoding); err != nil || encoding != "JSON_STRING" {
			t.Fatalf("encoding %q: %v", encoding, err)
		}
		var count int
		if err := d.DB.QueryRow(`SELECT COUNT(*) FROM trip_names JOIN trips ON trip_names.trip_id=trips.id WHERE trips.trip='trip'`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("links count %d: %v", count, err)
		}
		if _, err := d.DB.Exec(`INSERT INTO trips(type,trip,created_on) VALUES('PEST',NULL,1)`); err != nil {
			t.Fatal(err)
		}
		var nextID int64
		if err := d.DB.QueryRow(`SELECT MAX(id) FROM trips`).Scan(&nextID); err != nil || nextID <= 100 {
			t.Fatalf("autoincrement sequence lost: id=%d err=%v", nextID, err)
		}
		if _, err := d.DB.Exec(`INSERT INTO trips(type,trip,created_on) VALUES('INVALID',NULL,1)`); err == nil {
			t.Fatal("invalid role accepted")
		}
		if err := d.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE type='index' AND name='custom_trip_created'`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("custom index count %d: %v", count, err)
		}
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSchemaUpgradeFailureRollsBackDDLAndData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	seedLegacySQLite(t, path, true)
	if d, err := Open(context.Background(), Config{Path: path}); err == nil {
		d.Close()
		t.Fatal("orphan data accepted")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name='visibility'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("DDL not rolled back: %d %v", count, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM trip_names`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("data not preserved: %d %v", count, err)
	}
	if _, err := db.Exec(`INSERT INTO trips(type,trip,created_on) VALUES('PEST','pest',1)`); err == nil {
		t.Fatal("constraint upgrade was not rolled back")
	}
}

func TestSchemaRejectsFutureVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	d, err := Open(context.Background(), Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.DB.Exec(`INSERT INTO schema_version VALUES(?)`, currentSchemaVersion+1); err != nil {
		t.Fatal(err)
	}
	d.Close()
	if d, err = Open(context.Background(), Config{Path: path}); err == nil {
		d.Close()
		t.Fatal("future schema accepted")
	}
}

func TestSchemaUpgradeRejectsExtraTripColumnsWithoutDataLoss(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.db")
	seedLegacySQLite(t, path, false)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE trips ADD COLUMN custom_note TEXT; UPDATE trips SET custom_note='keep this'`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if d, err := Open(context.Background(), Config{Path: path}); err == nil {
		d.Close()
		t.Fatal("custom column silently removed")
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var note string
	if err := db.QueryRow(`SELECT custom_note FROM trips WHERE id=3`).Scan(&note); err != nil || note != "keep this" {
		t.Fatalf("custom data=%q: %v", note, err)
	}
}

func TestSchemaSummaryIdentityCannotBeNull(t *testing.T) {
	d := openTestDB(t)
	if _, err := d.DB.Exec(`INSERT INTO agent_memory_summary(identity_key,content,covered_through_id,fingerprint,created_on,expires_on) VALUES(NULL,'summary',1,'f',1,2)`); err == nil {
		t.Fatal("NULL summary identity accepted")
	}
}

func TestSchemaUpgradesLegacySummaryPrimaryKey(t *testing.T) {
	for _, nullIdentity := range []bool{false, true} {
		t.Run(fmt.Sprint(nullIdentity), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "summary.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(`CREATE TABLE agent_memory_summary(identity_key TEXT PRIMARY KEY,content TEXT NOT NULL,covered_through_id INTEGER NOT NULL,fingerprint TEXT NOT NULL,created_on INTEGER NOT NULL,expires_on INTEGER NOT NULL); CREATE INDEX custom_summary_created ON agent_memory_summary(created_on); INSERT INTO agent_memory_summary VALUES('key','keep',7,'fingerprint',1,9)`)
			if err != nil {
				t.Fatal(err)
			}
			if nullIdentity {
				if _, err = db.Exec(`INSERT INTO agent_memory_summary VALUES(NULL,'invalid',7,'f',1,9)`); err != nil {
					t.Fatal(err)
				}
			}
			db.Close()
			d, err := Open(context.Background(), Config{Path: path})
			if nullIdentity {
				if err == nil {
					d.Close()
					t.Fatal("null legacy summary accepted")
				}
				db, err = sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				var count int
				if err = db.QueryRow(`SELECT COUNT(*) FROM agent_memory_summary`).Scan(&count); err != nil || count != 2 {
					t.Fatalf("legacy data lost: %d %v", count, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			var content string
			if err = d.DB.QueryRow(`SELECT content FROM agent_memory_summary WHERE identity_key='key'`).Scan(&content); err != nil || content != "keep" {
				t.Fatalf("summary=%q: %v", content, err)
			}
			if _, err = d.DB.Exec(`INSERT INTO agent_memory_summary VALUES(NULL,'bad',7,'f',1,9)`); err == nil {
				t.Fatal("legacy summary accepts null identity")
			}
			var count int
			if err = d.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name='custom_summary_created'`).Scan(&count); err != nil || count != 1 {
				t.Fatalf("index count=%d: %v", count, err)
			}
		})
	}
}
