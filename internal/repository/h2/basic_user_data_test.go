package h2

import (
	"context"
	"strings"
	"testing"
)

func TestBasicUserDataUsesTripOrHashAndRendersSaturnPayload(t *testing.T) {
	d := openTestDB(t)
	for _, statement := range []string{
		"INSERT INTO messages(trip,name,hash,message,created_on,visibility) VALUES('trip-a','nick-a','hash-a','one',1,'PUBLIC')",
		"INSERT INTO messages(trip,name,hash,message,created_on,visibility) VALUES('trip-a','nick-b','hash-b','two',2,'PUBLIC')",
		"INSERT INTO messages(trip,name,hash,message,created_on,visibility) VALUES('trip-b','nick-c','hash-c','three',3,'PUBLIC')",
	} {
		if _, err := d.DB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	data, err := d.BasicUserData(context.Background(), "hash-a", "trip-a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(data, "Hashes: \\n") || !strings.Contains(data, "Nicks: \\n") || !strings.HasSuffix(data, " \\n") {
		t.Fatalf("payload=%q", data)
	}
	for _, want := range []string{"hash-a", "hash-b", "nick-a", "nick-b"} {
		if !strings.Contains(data, want) {
			t.Fatalf("payload=%q missing %q", data, want)
		}
	}
	if strings.Contains(data, "hash-c") || strings.Contains(data, "nick-c") {
		t.Fatalf("trip leaked other data: %q", data)
	}

	byHash, err := d.BasicUserData(context.Background(), "hash-c", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(byHash, "hash-c") || !strings.Contains(byHash, "nick-c") {
		t.Fatalf("hash fallback=%q", byHash)
	}
	missing, err := d.BasicUserData(context.Background(), "absent", "")
	if err != nil {
		t.Fatal(err)
	}
	if missing != "Hashes: \\n \\nNicks: \\n \\n" {
		t.Fatalf("absent payload=%q", missing)
	}
}
