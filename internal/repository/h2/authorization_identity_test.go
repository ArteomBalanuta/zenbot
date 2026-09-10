package h2

import (
	"context"
	"testing"

	"zenbot/internal/model"
)

func TestGrantTripInsertsAndUpdatesAllPersistedRoles(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	for _, role := range []model.Role{model.ADMIN, model.MODERATOR, model.TRUSTED, model.USER, model.REGULAR, model.PEST} {
		trip := "trip-" + role.String()
		if err := d.GrantTrip(ctx, trip, role); err != nil {
			t.Fatal(err)
		}
		got, err := d.ResolveRole(ctx, trip)
		if err != nil || got != role {
			t.Fatalf("trip=%s got=%v err=%v want=%v", trip, got, err, role)
		}
	}
	if err := d.GrantTrip(ctx, "trip-Admin", model.PEST); err != nil {
		t.Fatal(err)
	}
	got, err := d.ResolveRole(ctx, "trip-Admin")
	if err != nil || got != model.PEST {
		t.Fatalf("updated role=%v err=%v", got, err)
	}
}

func TestAuthorizationConfiguredWildcardAndThreshold(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if err := d.GrantTrip(ctx, "trusted", model.TRUSTED); err != nil {
		t.Fatal(err)
	}
	allowed, err := d.IsTripAuthorized(ctx, "trusted", model.USER, nil)
	if err != nil || !allowed {
		t.Fatalf("trusted should satisfy USER: allowed=%v err=%v", allowed, err)
	}
	allowed, err = d.IsTripAuthorized(ctx, "unknown", model.ADMIN, []string{" x "})
	if err != nil || !allowed {
		t.Fatalf("wildcard should authorize: allowed=%v err=%v", allowed, err)
	}
	allowed, err = d.IsTripAuthorized(ctx, "unknown", model.ADMIN, nil)
	if err != nil || allowed {
		t.Fatalf("unknown should fail closed: allowed=%v err=%v", allowed, err)
	}
}
