package h2

import (
	"context"
	"strings"
	"testing"

	"zenbot/internal/repository"
)

func TestDescribeAgentSchemaIncludesKeysIndexesAndColumnMetadata(t *testing.T) {
	d := openTestDB(t)
	_, err := d.DB.Exec(`
		CREATE TABLE agent_schema_parent (id INTEGER PRIMARY KEY, label VARCHAR(80) NOT NULL);
		CREATE TABLE agent_schema_child (
			id INTEGER PRIMARY KEY,
			parent_id INTEGER NOT NULL,
			optional_note VARCHAR(100),
			CONSTRAINT fk_agent_schema_parent FOREIGN KEY (parent_id) REFERENCES agent_schema_parent(id)
		);
		CREATE UNIQUE INDEX ux_agent_schema_child_note ON agent_schema_child(optional_note);
	`)
	if err != nil {
		t.Fatal(err)
	}

	schema, err := d.DescribeAgentSchema(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	child, ok := schema.FindTable("AGENT_SCHEMA_CHILD")
	if !ok {
		t.Fatalf("agent_schema_child missing from %#v", schema.TableNames())
	}
	if len(child.Columns) != 3 {
		t.Fatalf("columns=%#v", child.Columns)
	}
	if got := child.Columns[0]; got.Ordinal != 0 || !strings.EqualFold(got.Name, "id") || !got.PrimaryKey || got.Nullable {
		t.Fatalf("primary key metadata=%#v", got)
	}
	if got := child.Columns[2]; got.Ordinal != 2 || !strings.EqualFold(got.Name, "optional_note") || !got.Nullable {
		t.Fatalf("nullable column metadata=%#v", got)
	}
	if len(child.Indexes) == 0 || !containsSchemaIndex(child.Indexes, "UX_AGENT_SCHEMA_CHILD_NOTE", true, []string{"OPTIONAL_NOTE"}) {
		t.Fatalf("indexes=%#v", child.Indexes)
	}
	if len(child.ForeignKeys) != 1 {
		t.Fatalf("foreign keys=%#v", child.ForeignKeys)
	}
	fk := child.ForeignKeys[0]
	if !strings.EqualFold(fk.ReferencedTable, "agent_schema_parent") || !strings.EqualFold(fk.FromColumn, "parent_id") || !strings.EqualFold(fk.ToColumn, "id") {
		t.Fatalf("foreign key=%#v", fk)
	}
}

func containsSchemaIndex(indexes []repository.AgentDatabaseIndex, name string, unique bool, columns []string) bool {
	for _, index := range indexes {
		if strings.EqualFold(index.Name, name) && index.Unique == unique && len(index.Columns) == len(columns) {
			matches := true
			for i := range columns {
				matches = matches && strings.EqualFold(index.Columns[i], columns[i])
			}
			if matches {
				return true
			}
		}
	}
	return false
}
