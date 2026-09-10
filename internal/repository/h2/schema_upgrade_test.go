package h2

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestBootstrapUpgradesLegacyVisibilityAndTripRoleConstraint(t *testing.T) {
	database := openTestDB(t)
	for _, index := range []string{
		"idx_agent_messages_name_room_visibility_created",
		"idx_agent_messages_name_visibility_created",
		"idx_agent_messages_room_visibility_created",
		"idx_agent_messages_trip_visibility_created",
		"idx_agent_messages_visibility",
	} {
		if _, err := database.DB.Exec("DROP INDEX IF EXISTS " + index); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.DB.Exec(`ALTER TABLE messages DROP COLUMN visibility`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.Exec(`INSERT INTO messages(name,message,created_on) VALUES('legacy','row',1)`); err != nil {
		t.Fatal(err)
	}
	constraints, err := tripCheckConstraints(context.Background(), database.DB)
	if err != nil {
		t.Fatal(err)
	}
	for _, constraint := range constraints {
		quoted := `"` + strings.ReplaceAll(constraint, `"`, `""`) + `"`
		if _, err := database.DB.Exec("ALTER TABLE trips DROP CONSTRAINT " + quoted); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.DB.Exec(`ALTER TABLE trips ADD CONSTRAINT old_trip_type_check CHECK (type IN ('ADMIN','MODERATOR','TRUSTED','USER','REGULAR'))`); err != nil {
		t.Fatal(err)
	}

	if err := bootstrap(context.Background(), database.DB); err != nil {
		t.Fatal(err)
	}

	var visibility string
	if err := database.DB.QueryRow(`SELECT visibility FROM messages WHERE name='legacy'`).Scan(&visibility); err != nil || visibility != "PUBLIC" {
		t.Fatalf("visibility=%q err=%v", visibility, err)
	}
	if _, err := database.DB.Exec(`INSERT INTO messages(name,message,created_on) VALUES('defaulted','row',2)`); err != nil {
		t.Fatalf("visibility default missing: %v", err)
	}
	if _, err := database.DB.Exec(`INSERT INTO trips(type,trip,created_on) VALUES('PEST','pest-trip',1)`); err != nil {
		t.Fatalf("PEST role remains blocked: %v", err)
	}
	var version int
	if err := database.DB.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version); err != nil || version < currentSchemaVersion {
		t.Fatalf("schema version=%d err=%v current=%d", version, err, currentSchemaVersion)
	}
	var indexCount int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM information_schema.indexes WHERE LOWER(index_name)='idx_agent_messages_visibility'`).Scan(&indexCount); err != nil || indexCount != 1 {
		t.Fatalf("visibility index count=%d err=%v", indexCount, err)
	}
}

func TestTripCheckUpgradeHelperQuotesConstraintNames(t *testing.T) {
	if got := quoteIdentifier(`odd"name`); got != `"odd""name"` {
		t.Fatalf("quoted=%s", fmt.Sprintf("%q", got))
	}
}
