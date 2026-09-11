package sqlite

import (
	"context"
	"fmt"

	"zenbot/internal/repository"
)

// DescribeAgentSchema reads application tables from the main SQLite database.
// SQLite internal tables and attached databases are excluded.
func (d *Database) DescribeAgentSchema(ctx context.Context) (repository.AgentDatabaseSchema, error) {
	if d == nil || d.DB == nil {
		return repository.AgentDatabaseSchema{}, fmt.Errorf("agent schema database unavailable")
	}
	tables, err := d.DB.QueryContext(ctx, `SELECT name FROM main.sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return repository.AgentDatabaseSchema{}, err
	}
	tableNames := []string{}
	for tables.Next() {
		var name string
		if err := tables.Scan(&name); err != nil {
			tables.Close()
			return repository.AgentDatabaseSchema{}, err
		}
		tableNames = append(tableNames, name)
	}
	if err := tables.Err(); err != nil {
		tables.Close()
		return repository.AgentDatabaseSchema{}, err
	}
	if err := tables.Close(); err != nil {
		return repository.AgentDatabaseSchema{}, err
	}
	out := repository.AgentDatabaseSchema{}
	for _, name := range tableNames {
		columns, err := d.agentSchemaColumns(ctx, name)
		if err != nil {
			return out, fmt.Errorf("inspect columns for %s: %w", name, err)
		}
		indexes, err := d.agentSchemaIndexes(ctx, name)
		if err != nil {
			return out, fmt.Errorf("inspect indexes for %s: %w", name, err)
		}
		foreignKeys, err := d.agentSchemaForeignKeys(ctx, name)
		if err != nil {
			return out, fmt.Errorf("inspect foreign keys for %s: %w", name, err)
		}
		out.Tables = append(out.Tables, repository.AgentDatabaseTable{Name: name, Columns: columns, Indexes: indexes, ForeignKeys: foreignKeys})
	}
	return out, nil
}

func (d *Database) agentSchemaColumns(ctx context.Context, table string) ([]repository.AgentDatabaseColumn, error) {
	rows, err := d.DB.QueryContext(ctx, `
		SELECT cid, name, type, "notnull", pk
		FROM pragma_table_xinfo(?1, 'main') ORDER BY cid`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []repository.AgentDatabaseColumn{}
	for rows.Next() {
		var c repository.AgentDatabaseColumn
		var notNull, primaryKey int
		if err := rows.Scan(&c.Ordinal, &c.Name, &c.Type, &notNull, &primaryKey); err != nil {
			return nil, err
		}
		c.PrimaryKey = primaryKey > 0
		c.Nullable = notNull == 0 && !c.PrimaryKey
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *Database) agentSchemaIndexes(ctx context.Context, table string) ([]repository.AgentDatabaseIndex, error) {
	rows, err := d.DB.QueryContext(ctx, `
		SELECT i.name, i."unique", COALESCE(ic.name, '<expression>')
		FROM pragma_index_list(?1, 'main') i
		JOIN pragma_index_xinfo(i.name, 'main') ic ON ic."key"=1
		ORDER BY i.name, ic.seqno`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []repository.AgentDatabaseIndex{}
	byName := map[string]int{}
	for rows.Next() {
		var name, column string
		var unique bool
		if err := rows.Scan(&name, &unique, &column); err != nil {
			return nil, err
		}
		i, ok := byName[name]
		if !ok {
			i = len(out)
			byName[name] = i
			out = append(out, repository.AgentDatabaseIndex{Name: name, Unique: unique})
		}
		out[i].Columns = append(out[i].Columns, column)
	}
	return out, rows.Err()
}

func (d *Database) agentSchemaForeignKeys(ctx context.Context, table string) ([]repository.AgentDatabaseForeignKey, error) {
	rows, err := d.DB.QueryContext(ctx, `
		SELECT id, seq, "table", "from", COALESCE("to", ''), on_update, on_delete, "match"
		FROM pragma_foreign_key_list(?1, 'main') ORDER BY id, seq`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []repository.AgentDatabaseForeignKey{}
	for rows.Next() {
		var fk repository.AgentDatabaseForeignKey
		if err := rows.Scan(&fk.ID, &fk.Sequence, &fk.ReferencedTable, &fk.FromColumn, &fk.ToColumn, &fk.OnUpdate, &fk.OnDelete, &fk.Match); err != nil {
			return nil, err
		}
		out = append(out, fk)
	}
	return out, rows.Err()
}

var _ repository.AgentSchemaRepository = (*Database)(nil)
