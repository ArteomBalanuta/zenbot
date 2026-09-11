package sqlite

import (
	"context"
	"testing"

	"zenbot/internal/model"
)

func TestIdentityAuditTripLookupDoesNotFoldCredentials(t *testing.T) {
	database := openTestDB(t)
	if _, err := database.DB.Exec(`INSERT INTO trips(type,trip,created_on) VALUES('REGULAR','AbC123',1)`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		trip string
		want bool
	}{{"AbC123", true}, {"abc123", false}} {
		found, err := database.IsTripRegistered(context.Background(), tc.trip)
		if err != nil || found != tc.want {
			t.Fatalf("IsTripRegistered(%q)=%t, %v; want %t", tc.trip, found, err, tc.want)
		}
	}
}

func TestIdentityAuditAliasCannotAttachToCaseVariantTrip(t *testing.T) {
	database := openTestDB(t)
	if _, err := database.DB.Exec(`INSERT INTO trips(type,trip,created_on) VALUES('REGULAR','AbC123',1)`); err != nil {
		t.Fatal(err)
	}
	err := database.RegisterNameByTrip(context.Background(), "new-alias", "abc123")
	var names, links int
	if queryErr := database.DB.QueryRow(`SELECT COUNT(*) FROM names WHERE name='new-alias'`).Scan(&names); queryErr != nil {
		t.Fatal(queryErr)
	}
	if queryErr := database.DB.QueryRow(`SELECT COUNT(*) FROM trip_names`).Scan(&links); queryErr != nil {
		t.Fatal(queryErr)
	}
	if err == nil || names != 0 || links != 0 {
		t.Fatalf("case-variant credential acquired alias: err=%v names=%d links=%d", err, names, links)
	}
}

func TestIdentityAuditConfiguredAdminTripIsExact(t *testing.T) {
	database := openTestDB(t)
	for _, tc := range []struct {
		trip string
		want bool
	}{{"AbC123", true}, {"abc123", false}} {
		allowed, err := database.IsTripAuthorized(context.Background(), tc.trip, model.ADMIN, []string{"AbC123"})
		if err != nil || allowed != tc.want {
			t.Fatalf("configured administrator %q authorized=%t, %v; want %t", tc.trip, allowed, err, tc.want)
		}
	}
}
