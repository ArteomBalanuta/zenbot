package h2

import (
	"context"
	"fmt"

	"zenbot/internal/repository"
)

// DescribeAgentSchema reads complete H2 metadata through the active database
// boundary. It intentionally restricts discovery to the application PUBLIC
// schema, so model-visible SQL cannot learn about server internals.
func (d *Database) DescribeAgentSchema(ctx context.Context) (repository.AgentDatabaseSchema, error) {
	if d == nil || d.DB == nil {
		return repository.AgentDatabaseSchema{}, fmt.Errorf("agent schema database unavailable")
	}
	tables, err := d.DB.QueryContext(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name`)
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
		SELECT ordinal_position, column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema='public' AND table_name=$1
		ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []repository.AgentDatabaseColumn{}
	for rows.Next() {
		var c repository.AgentDatabaseColumn
		var nullable string
		if err := rows.Scan(&c.Ordinal, &c.Name, &c.Type, &nullable); err != nil {
			return nil, err
		}
		c.Ordinal-- // Saturn exposes zero-based ordinals.
		c.Nullable = nullable == "YES" && !c.PrimaryKey
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	primaryKeys, err := d.agentSchemaPrimaryKeys(ctx, table)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].PrimaryKey = primaryKeys[out[i].Name]
		if out[i].PrimaryKey {
			out[i].Nullable = false
		}
	}
	return out, nil
}

func (d *Database) agentSchemaPrimaryKeys(ctx context.Context, table string) (map[string]bool, error) {
	rows, err := d.DB.QueryContext(ctx, `
		SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_schema=kcu.constraint_schema AND tc.constraint_name=kcu.constraint_name
		WHERE tc.table_schema='public' AND tc.table_name=$1 AND tc.constraint_type='PRIMARY KEY'`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

func (d *Database) agentSchemaIndexes(ctx context.Context, table string) ([]repository.AgentDatabaseIndex, error) {
	rows, err := d.DB.QueryContext(ctx, `
		SELECT i.index_name, i.index_type_name, ic.column_name
		FROM information_schema.indexes i
		JOIN information_schema.index_columns ic ON i.index_schema=ic.index_schema AND i.index_name=ic.index_name
		WHERE i.table_schema='public' AND i.table_name=$1
		ORDER BY i.index_name, ic.ordinal_position`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []repository.AgentDatabaseIndex{}
	byName := map[string]int{}
	for rows.Next() {
		var name, kind, column string
		if err := rows.Scan(&name, &kind, &column); err != nil {
			return nil, err
		}
		i, ok := byName[name]
		if !ok {
			i = len(out)
			byName[name] = i
			out = append(out, repository.AgentDatabaseIndex{Name: name, Unique: kind == "UNIQUE INDEX"})
		}
		out[i].Columns = append(out[i].Columns, column)
	}
	return out, rows.Err()
}

func (d *Database) agentSchemaForeignKeys(ctx context.Context, table string) ([]repository.AgentDatabaseForeignKey, error) {
	rows, err := d.DB.QueryContext(ctx, `
		SELECT kcu.ordinal_position, ccu.table_name, kcu.column_name, ccu.column_name,
		       rc.update_rule, rc.delete_rule, rc.match_option
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_schema=kcu.constraint_schema AND tc.constraint_name=kcu.constraint_name
		JOIN information_schema.referential_constraints rc
		  ON tc.constraint_schema=rc.constraint_schema AND tc.constraint_name=rc.constraint_name
		JOIN information_schema.constraint_column_usage ccu
		  ON rc.unique_constraint_schema=ccu.constraint_schema AND rc.unique_constraint_name=ccu.constraint_name
		WHERE tc.table_schema='public' AND tc.table_name=$1 AND tc.constraint_type='FOREIGN KEY'
		ORDER BY tc.constraint_name, kcu.ordinal_position`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []repository.AgentDatabaseForeignKey{}
	for rows.Next() {
		var fk repository.AgentDatabaseForeignKey
		if err := rows.Scan(&fk.Sequence, &fk.ReferencedTable, &fk.FromColumn, &fk.ToColumn, &fk.OnUpdate, &fk.OnDelete, &fk.Match); err != nil {
			return nil, err
		}
		fk.ID = len(out) + 1
		out = append(out, fk)
	}
	return out, rows.Err()
}

var _ repository.AgentSchemaRepository = (*Database)(nil)
