package h2_test

import (
	"context"
	"net"
	"testing"

	"zenbot/internal/repository/h2"
	"zenbot/internal/testutil/h2fixture"
)

func TestAutoPortFixturesAreConcurrentAndIsolated(t *testing.T) {
	a := h2fixture.Open(t, "a")
	b := h2fixture.Open(t, "b")
	if a.Server.Addr() == b.Server.Addr() {
		t.Fatalf("fixture addresses match: %s", a.Server.Addr())
	}
	for _, tc := range []struct {
		db   *h2.Database
		name string
	}{
		{a, "sentinel-a"},
		{b, "sentinel-b"},
	} {
		if _, err := tc.db.DB.Exec("INSERT INTO messages(name,created_on,visibility) VALUES($1,$2,$3)", tc.name, int64(1), "PUBLIC"); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		db, other *h2.Database
		name      string
	}{
		{a, b, "sentinel-a"},
		{b, a, "sentinel-b"},
	} {
		var own, other int
		if err := tc.db.DB.QueryRow("SELECT COUNT(*) FROM messages WHERE name=$1", tc.name).Scan(&own); err != nil {
			t.Fatal(err)
		}
		if err := tc.other.DB.QueryRow("SELECT COUNT(*) FROM messages WHERE name=$1", tc.name).Scan(&other); err != nil {
			t.Fatal(err)
		}
		if own != 1 || other != 0 {
			t.Fatalf("%s counts own=%d other=%d", tc.name, own, other)
		}
	}
}

func TestAutoPortFixtureCloseDoesNotInterruptSibling(t *testing.T) {
	a := h2fixture.Open(t, "a")
	b := h2fixture.Open(t, "b")
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.DB.PingContext(context.Background()); err != nil {
		t.Fatalf("sibling ping after close: %v", err)
	}
	if _, err := b.DB.Exec("INSERT INTO messages(name,created_on,visibility) VALUES($1,$2,$3)", "survives", int64(1), "PUBLIC"); err != nil {
		t.Fatalf("sibling insert after close: %v", err)
	}
}

func TestAutoPortDoesNotAdoptOccupiedLegacyPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	d := h2fixture.Open(t, "occupied")
	if got, occupied := d.Server.Addr(), listener.Addr().String(); got == occupied {
		t.Fatalf("AutoPort adopted occupied listener %s", got)
	}
	if err := d.DB.PingContext(context.Background()); err != nil {
		t.Fatalf("fixture ping: %v", err)
	}
}
