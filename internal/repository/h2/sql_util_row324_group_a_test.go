package h2

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"zenbot/internal/model"
)

func TestGroupA_TripRoleContracts(t *testing.T) {
	const insertConstant = "INSERT_INTO_TRIPS_TYPE_TRIP_CREATED_ON_VALUES"
	const updateConstant = "UPDATE_TRIPS_SET_TYPE_WHERE_TRIP"
	d := openTestDB(t)
	ctx := context.Background()
	roles := []model.Role{model.ADMIN, model.MODERATOR, model.TRUSTED, model.USER, model.REGULAR, model.PEST}
	for i, role := range roles {
		trip := "role-trip-" + string(rune('a'+i))
		if err := d.GrantTrip(ctx, trip, role); err != nil {
			t.Fatalf("%s: insert %v: %v", insertConstant, role, err)
		}
		got, err := d.ResolveRole(ctx, trip)
		if err != nil || got != role {
			t.Fatalf("%s: got=%v err=%v want=%v", insertConstant, got, err, role)
		}
		if err := d.GrantTrip(ctx, trip, roles[(i+1)%len(roles)]); err != nil {
			t.Fatalf("%s: update %v: %v", updateConstant, role, err)
		}
		got, err = d.ResolveRole(ctx, trip)
		if err != nil || got != roles[(i+1)%len(roles)] {
			t.Fatalf("%s: got=%v err=%v", updateConstant, got, err)
		}
	}
	if err := d.GrantTrip(ctx, " ", model.USER); err == nil {
		t.Fatal("blank trip unexpectedly succeeded")
	}
	if err := d.GrantTrip(ctx, "bad-role", model.Role(99)); err == nil {
		t.Fatal("invalid role unexpectedly succeeded")
	}
	if got, err := d.ResolveRole(ctx, " "); err != nil || got != model.REGULAR {
		t.Fatalf("blank resolve got=%v err=%v", got, err)
	}
	if got, err := d.ResolveRole(ctx, "unknown"); err != nil || got != model.REGULAR {
		t.Fatalf("unknown resolve got=%v err=%v", got, err)
	}
	var count int
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM trips WHERE trip='bad-role'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid role left rows: count=%d err=%v", count, err)
	}
	if _, err := roleFromDatabaseName("INVALID"); err == nil {
		t.Fatal("invalid persisted role unexpectedly resolved")
	}
	if err := d.WithTx(ctx, func(tx *sql.Tx) error {
		if _, e := tx.Exec("INSERT INTO trips(type,trip,created_on) VALUES('USER','rollback-role',1)"); e != nil {
			return e
		}
		return errors.New("rollback")
	}); err == nil {
		t.Fatal("expected rollback error")
	}
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM trips WHERE trip='rollback-role'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback left rows: count=%d err=%v", count, err)
	}
}

func TestGroupA_IdentityRegistrationContracts(t *testing.T) {
	const namesConstant = "INSERT_NAMES"
	const tripsConstant = "INSERT_TRIPS"
	const linkConstant = "INSERT_TRIP_NAME"
	d := openTestDB(t)
	if err := d.Register(" Alice ", " trip-a ", model.REGULAR); err != nil {
		t.Fatalf("%s/%s/%s: %v", namesConstant, tripsConstant, linkConstant, err)
	}
	var nameID, tripID, links int64
	if err := d.DB.QueryRow("SELECT id FROM names WHERE name='Alice'").Scan(&nameID); err != nil || nameID <= 0 {
		t.Fatalf("name id=%d err=%v", nameID, err)
	}
	if err := d.DB.QueryRow("SELECT id FROM trips WHERE trip='trip-a'").Scan(&tripID); err != nil || tripID <= 0 {
		t.Fatalf("trip id=%d err=%v", tripID, err)
	}
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM trip_names WHERE name_id=? AND trip_id=?", nameID, tripID).Scan(&links); err != nil || links != 1 {
		t.Fatalf("link count=%d err=%v", links, err)
	}
	if err := d.RegisterNameByTrip("O'Neil", "TRIP-A"); err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterTripByName("Alice", "trip-b"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, trip string }{{"", "trip-a"}, {"Bob", ""}} {
		if err := d.RegisterNameByTrip(tc.name, tc.trip); err == nil {
			t.Fatalf("blank RegisterNameByTrip(%q,%q) succeeded", tc.name, tc.trip)
		}
	}
	if err := d.RegisterTripByName("", "trip-c"); err == nil {
		t.Fatal("blank RegisterTripByName name succeeded")
	}
	if err := d.Register(" Alice ", "trip-c", model.REGULAR); err == nil {
		t.Fatal("duplicate name succeeded")
	}
	if err := d.Register("Bob", "trip-a", model.REGULAR); err == nil {
		t.Fatal("duplicate trip succeeded")
	}
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM names").Scan(&nameID); err != nil || nameID != 2 {
		t.Fatalf("names=%d err=%v", nameID, err)
	}
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM trips").Scan(&tripID); err != nil || tripID != 2 {
		t.Fatalf("trips=%d err=%v", tripID, err)
	}
}

func TestGroupA_CommandAuditContract(t *testing.T) {
	const constant = "INSERT_INTO_EXECUTED_COMMANDS_TRIP_COMMAND_NAME_ARGUMENTS_STATUS_CREATED_ON_VALUES"
	d := openTestDB(t)
	r := model.CommandAuditRecord{Trip: "trip", CommandName: "say", Arguments: "it's quoted", Status: "SUCCESSFUL", CreatedOnMillis: 42, Channel: "chan"}
	id, err := d.CommandAudit(context.Background(), r)
	if err != nil || id <= 0 {
		t.Fatalf("%s id=%d err=%v", constant, id, err)
	}
	var got model.CommandAuditRecord
	if err := d.DB.QueryRow("SELECT trip,command_name,arguments,status,created_on,channel FROM executed_commands WHERE id=?", id).Scan(&got.Trip, &got.CommandName, &got.Arguments, &got.Status, &got.CreatedOnMillis, &got.Channel); err != nil {
		t.Fatal(err)
	}
	if got != r {
		t.Fatalf("got=%+v want=%+v", got, r)
	}
	if _, err := d.CommandAudit(context.Background(), model.CommandAuditRecord{Trip: "", CommandName: "", Arguments: "", Status: "", CreatedOnMillis: 43, Channel: ""}); err != nil {
		t.Fatal(err)
	}
}

func TestGroupA_MessageAuditContract(t *testing.T) {
	const constant = "INSERT_INTO_MESSAGES"
	d := openTestDB(t)
	ctx := context.Background()
	id, err := d.MessageAudit(ctx, model.MessageRecord{Trip: "trip", Name: "name", Hash: "hash", Message: "it's quoted", CreatedOnMillis: 7, Channel: "chan"})
	if err != nil || id <= 0 {
		t.Fatalf("%s id=%d err=%v", constant, id, err)
	}
	var visibility string
	if err := d.DB.QueryRow("SELECT visibility FROM messages WHERE id=?", id).Scan(&visibility); err != nil || visibility != "PUBLIC" {
		t.Fatalf("default visibility=%q err=%v", visibility, err)
	}
	if _, err := d.MessageAudit(ctx, model.MessageRecord{Name: "name", Message: "secret", CreatedOnMillis: 8, Visibility: "WHISPER"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.MessageAudit(ctx, model.MessageRecord{Message: "bad", CreatedOnMillis: 9, Visibility: "INVALID"}); err == nil {
		t.Fatal("invalid visibility succeeded")
	}
	if _, err := d.MessageAudit(ctx, model.MessageRecord{Name: "", Message: "empty optional fields", CreatedOnMillis: 10, Visibility: "PUBLIC"}); err != nil {
		t.Fatalf("nullable optional fields: %v", err)
	}
}

func TestGroupA_NicksByTripContract(t *testing.T) {
	const constant = "GET_NICKS_BY_TRIP"
	d := openTestDB(t)
	for _, nick := range []string{"alice", "alice", "bob"} {
		if _, err := d.DB.Exec("INSERT INTO messages(trip,name,message,created_on) VALUES(?,?,?,?)", "Trip-X", nick, "m", 1); err != nil {
			t.Fatal(err)
		}
	}
	got, err := d.NicksByTrip(context.Background(), "TRIP-X")
	if err != nil {
		t.Fatalf("%s: %v", constant, err)
	}
	if len(got) != 2 {
		t.Fatalf("distinct nicks=%v", got)
	}
	empty, err := d.NicksByTrip(context.Background(), "no-such-trip")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty=%v err=%v", empty, err)
	}
	d.DB.Close()
	if _, err := d.NicksByTrip(context.Background(), "trip-x"); err == nil {
		t.Fatal("closed database did not propagate query error")
	}
}

func TestGroupA_SelectRoleByTripContract(t *testing.T) {
	const constant = "SELECT_ROLE_BY_TRIP"
	d := openTestDB(t)
	if err := d.GrantTrip(context.Background(), "role", model.MODERATOR); err != nil {
		t.Fatal(err)
	}
	got, err := d.ResolveRole(context.Background(), "role")
	if err != nil || got != model.MODERATOR {
		t.Fatalf("%s got=%v err=%v", constant, got, err)
	}
	if got, err := d.ResolveRole(context.Background(), " "); err != nil || got != model.REGULAR {
		t.Fatalf("blank got=%v err=%v", got, err)
	}
}
