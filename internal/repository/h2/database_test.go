package h2

import "testing"

func TestRealH2PostgresWire(t *testing.T) {
	d := openTestDB(t)
	var version string
	if err := d.DB.QueryRow("SELECT H2VERSION()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version == "" {
		t.Fatal("empty H2 version")
	}
	if _, err := d.DB.Exec("INSERT INTO messages(trip,name,message,created_on,visibility) VALUES($1,$2,$3,$4,$5)", "t", "n", "m", int64(1), "PUBLIC"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM messages").Scan(&n); err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}
}
