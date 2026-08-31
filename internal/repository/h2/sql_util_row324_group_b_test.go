package h2

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/model"
	"zenbot/internal/repository"
)

func TestGroupBExactSaturnConstants(t *testing.T) {
	checks := map[string]string{
		"DELETE_TRIP_NAMES":           deleteTripNames,
		"DELETE_TRIP":                 deleteTrip,
		"DELETE_NAME":                 deleteName,
		"SELECT_NAME_TRIP_REGISTERED": selectNameTripRegistered,
		"SELECT_LAST_N_MESSAGES":      selectLastNMessages,
	}
	want := map[string]string{
		"DELETE_TRIP_NAMES":           "DELETE FROM trip_names WHERE trip_id IN (\n        SELECT id FROM trips WHERE trip = ?\n) OR name_id IN (\nSELECT id FROM names WHERE name = ?\n);",
		"DELETE_TRIP":                 "DELETE FROM trips WHERE trip = ?;",
		"DELETE_NAME":                 "DELETE FROM names WHERE name = ?;",
		"SELECT_NAME_TRIP_REGISTERED": "SELECT DISTINCT n.name,t.trip\nFROM trip_names tn\nINNER JOIN trips t on tn.trip_id = t.id\nINNER JOIN names n on tn.name_id = n.id ORDER BY t.trip DESC;",
		"SELECT_LAST_N_MESSAGES":      "SELECT name,trip,message,created_on FROM messages WHERE (name = ? or trip = ?) and visibility = 'PUBLIC' and (message not\nin ('LEFT','JOINED')) order by created_on desc,id desc limit ?;",
	}
	for name, got := range checks {
		if strings.TrimSpace(got) != strings.TrimSpace(want[name]) {
			t.Errorf("%s = %q, want %q", name, got, want[name])
		}
	}
}

func TestGroupBDeleteRequiresAuthorizedContextAndUsesTypedScope(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	seedIdentity(t, d, "alice", "trip-a")
	if _, err := d.DeleteIdentity(ctx, "alice", "trip-a"); !errors.Is(err, errSaturnUnauthorized) {
		t.Fatalf("unauthorized delete err=%v", err)
	}
	assertCounts(t, d, 1, 1, 1)
	result, err := d.DeleteIdentity(withSaturnAuthorization(ctx), "alice", "trip-a")
	if err != nil {
		t.Fatal(err)
	}
	if result != (repository.DeleteResult{TripNamesRows: 1, TripRows: 1, NameRows: 1}) {
		t.Fatalf("result=%+v", result)
	}
	assertCounts(t, d, 0, 0, 0)
}

func TestGroupBDeleteTripNamesHasORScopeAndParentsAreExact(t *testing.T) {
	d := openTestDB(t)
	seedIdentity(t, d, "shared", "trip-a")
	seedIdentity(t, d, "other", "trip-b")
	if _, err := d.DB.Exec("INSERT INTO trip_names(trip_id,name_id) SELECT t.id,n.id FROM trips t,names n WHERE t.trip=$1 AND n.name=$2", "trip-b", "shared"); err != nil {
		t.Fatal(err)
	}
	result, err := d.DeleteIdentity(withSaturnAuthorization(context.Background()), "shared", "trip-a")
	if err != nil || result.TripNamesRows != 2 || result.TripRows != 1 || result.NameRows != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	assertCounts(t, d, 1, 1, 1)
	var links int
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM trip_names").Scan(&links); err != nil || links != 1 {
		t.Fatalf("links=%d err=%v", links, err)
	}
}

func TestGroupBDeleteAbsentAndBlankAreNoOpWithoutParameterInjection(t *testing.T) {
	d := openTestDB(t)
	seedIdentity(t, d, "safe", "trip-safe")
	for _, input := range [][2]string{{"missing", "none"}, {"", ""}, {"' OR '1'='1", "x"}} {
		result, err := d.DeleteIdentity(withSaturnAuthorization(context.Background()), input[0], input[1])
		if err != nil || result != (repository.DeleteResult{}) {
			t.Fatalf("input=%q result=%+v err=%v", input, result, err)
		}
	}
	assertCounts(t, d, 1, 1, 1)
}

func TestGroupBDeleteRollsBackWhenParentStatementFails(t *testing.T) {
	d := openTestDB(t)
	seedIdentity(t, d, "alice", "trip-a")
	calls := 0
	exec := func(ctx context.Context, tx *sql.Tx, query string, args ...any) (sql.Result, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("injected parent failure")
		}
		return tx.ExecContext(ctx, query, args...)
	}
	if _, err := d.deleteIdentity(withSaturnAuthorization(context.Background()), "alice", "trip-a", exec); err == nil {
		t.Fatal("expected injected failure")
	}
	assertCounts(t, d, 1, 1, 1)
}

func TestGroupBSaturnRegisteredUsersAndExistingRegisteredUsersStayDistinct(t *testing.T) {
	d := openTestDB(t)
	seedIdentity(t, d, "zack", "trip-b")
	seedIdentity(t, d, "alice", "trip-a")
	got, err := d.SaturnRegisteredUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []repository.SaturnRegisteredUser{{Name: "zack", Trip: "trip-b"}, {Name: "alice", Trip: "trip-a"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got=%+v want=%+v", got, want)
	}
	legacy, err := d.RegisteredUsers(context.Background())
	if err != nil || len(legacy) != 2 || legacy[0].Trip != "trip-b" || legacy[0].Name != "zack" {
		t.Fatalf("legacy=%+v err=%v", legacy, err)
	}
}

func TestGroupBSaturnLastMessagesReturnsPublicRowsWithRowTripAndStableTies(t *testing.T) {
	d := openTestDB(t)
	rows := []struct {
		name, trip, msg string
		ts              int64
		vis             string
	}{
		{"alice", "trip-a", "old", 1, "PUBLIC"},
		{"alice", "trip-a", "whisper-secret", 3, "WHISPER"},
		{"bob", "trip-a", "first-at-tie", 5, "PUBLIC"},
		{"alice", "trip-b", "second-at-tie", 5, "PUBLIC"},
		{"alice", "trip-a", "LEFT", 6, "PUBLIC"},
	}
	for _, r := range rows {
		if _, err := d.DB.Exec("INSERT INTO messages(name,trip,message,created_on,visibility) VALUES(?,?,?,?,?)", r.name, r.trip, r.msg, r.ts, r.vis); err != nil {
			t.Fatal(err)
		}
	}
	name := "alice"
	got, err := d.SaturnLastMessages(context.Background(), &name, "trip-a", 5)
	if err != nil {
		t.Fatal(err)
	}
	want := []repository.SaturnLastMessage{
		{Name: "alice", Trip: "trip-b", Message: "second-at-tie", CreatedOn: 5},
		{Name: "bob", Trip: "trip-a", Message: "first-at-tie", CreatedOn: 5},
		{Name: "alice", Trip: "trip-a", Message: "old", CreatedOn: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got=%+v want=%+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got=%+v want=%+v", got, want)
		}
	}
	nilName, err := d.SaturnLastMessages(context.Background(), nil, "trip-a", 5)
	if err != nil || len(nilName) != 2 || nilName[0].Message != "first-at-tie" || nilName[1].Message != "old" {
		t.Fatalf("nullable name result=%+v err=%v", nilName, err)
	}
	public, err := d.LastMessages("alice", "trip-a", 5)
	if err != nil || len(public) != 3 || public[0].Message != "second-at-tie" {
		t.Fatalf("public=%+v err=%v", public, err)
	}
}

func TestGroupBAuthorizedSelectorResolvesUniqueCaseInsensitiveIdentity(t *testing.T) {
	d := openTestDB(t)
	seedIdentity(t, d, "Alice", "Trip-A")
	got, err := d.DeleteIdentityAuthorized(context.Background(), "  aLiCe  ")
	if err != nil || got.TripRows != 1 {
		t.Fatalf("result=%+v err=%v", got, err)
	}
	assertCounts(t, d, 0, 0, 0)
}

func TestGroupBAuthorizedSelectorRejectsBlankMissingAndAmbiguousWithoutDelete(t *testing.T) {
	d := openTestDB(t)
	seedIdentity(t, d, "alice", "trip-a")
	seedIdentity(t, d, "bob", "trip-b")
	for _, input := range []string{"", "missing"} {
		if got, err := d.DeleteIdentityAuthorized(context.Background(), input); err == nil || got != (repository.DeleteResult{}) {
			t.Fatalf("input=%q result=%+v err=%v", input, got, err)
		}
	}
	if _, err := d.DB.Exec("INSERT INTO trip_names(trip_id,name_id) SELECT t.id,n.id FROM trips t,names n WHERE t.trip=$1 AND n.name=$2", "trip-b", "alice"); err != nil {
		t.Fatal(err)
	}
	if got, err := d.DeleteIdentityAuthorized(context.Background(), "ALICE"); err == nil || got != (repository.DeleteResult{}) {
		t.Fatalf("ambiguous result=%+v err=%v", got, err)
	}
	assertCounts(t, d, 2, 2, 3)
}

func seedIdentity(t *testing.T, d *Database, name, trip string) {
	t.Helper()
	if err := d.Register(name, trip, model.REGULAR); err != nil {
		t.Fatal(err)
	}
}
func assertCounts(t *testing.T, d *Database, names, trips, links int) {
	t.Helper()
	for _, q := range []struct {
		sql  string
		want int
	}{{"SELECT COUNT(*) FROM names", names}, {"SELECT COUNT(*) FROM trips", trips}, {"SELECT COUNT(*) FROM trip_names", links}} {
		var got int
		if err := d.DB.QueryRow(q.sql).Scan(&got); err != nil || got != q.want {
			t.Fatalf("%s got=%d err=%v", q.sql, got, err)
		}
	}
}

var _ = sql.ErrNoRows
