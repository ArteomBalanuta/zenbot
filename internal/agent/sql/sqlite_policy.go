package sql

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"unicode/utf8"

	sqlast "github.com/rqlite/sql"
)

// SQLiteSelectPolicy validates a single SELECT against the application schema.
// The executor independently enforces read-only access on its connection.
type SQLiteSelectPolicy struct{ maxSQLChars int }

func NewSQLiteSelectPolicy(maxSQLChars int) SQLiteSelectPolicy {
	return SQLiteSelectPolicy{maxSQLChars: maxSQLChars}
}

func (p SQLiteSelectPolicy) Validate(query string, schema Schema) (ValidatedAgentSql, error) {
	if strings.TrimSpace(query) == "" {
		return ValidatedAgentSql{}, newPolicyError(EmptySQL, "SQL must not be blank", nil)
	}
	if schema == nil {
		return ValidatedAgentSql{}, newPolicyError(ForbiddenTable, "SQL schema is unavailable", nil)
	}
	if !utf8.ValidString(query) || strings.ContainsRune(query, 0) {
		return ValidatedAgentSql{}, newPolicyError(MalformedSQL, "SQL could not be parsed", errors.New("invalid SQL encoding"))
	}
	if p.maxSQLChars > 0 && utf8.RuneCountInString(query) > p.maxSQLChars {
		return ValidatedAgentSql{}, newPolicyError(SQLTooLong, "SQL exceeds the configured limit", nil)
	}
	statements, err := sqlast.NewParser(strings.NewReader(query)).ParseStatements()
	if err != nil {
		return ValidatedAgentSql{}, newPolicyError(MalformedSQL, "SQL could not be parsed", err)
	}
	if len(statements) != 1 {
		return ValidatedAgentSql{}, newPolicyError(ForbiddenStatement, "Exactly one SELECT statement is allowed", nil)
	}
	selectStmt, ok := statements[0].(*sqlast.SelectStatement)
	if !ok {
		return ValidatedAgentSql{}, newPolicyError(ForbiddenStatement, "Only SELECT statements are allowed", nil)
	}
	allowed := make(map[string]bool)
	for _, name := range schema.TableNames() {
		allowed[strings.ToLower(name)] = true
	}
	if _, err := sqlast.Walk(selectPolicyVisitor{allowed: allowed}, selectStmt); err != nil {
		return ValidatedAgentSql{}, err
	}
	sum := sha256.Sum256([]byte(query))
	return ValidatedAgentSql{SQL: query, Fingerprint: hex.EncodeToString(sum[:])}, nil
}

// A visitor value holds a lexical CTE scope. Each SELECT copies it before adding
// names, so a nested CTE cannot authorize an unrelated outer table reference.
type selectPolicyVisitor struct {
	allowed map[string]bool
	ctes    map[string]bool
}

func (v selectPolicyVisitor) Visit(node sqlast.Node) (sqlast.Visitor, sqlast.Node, error) {
	switch n := node.(type) {
	case *sqlast.Null:
		if n.X != nil {
			if _, err := sqlast.Walk(v, n.X); err != nil {
				return nil, node, err
			}
		}
	case sqlast.SelectExpr:
		if _, err := sqlast.Walk(v, n.SelectStatement); err != nil {
			return nil, node, err
		}
	case *sqlast.SelectExpr:
		if _, err := sqlast.Walk(v, n.SelectStatement); err != nil {
			return nil, node, err
		}
	case *sqlast.SelectStatement:
		if n.Values.IsValid() {
			return nil, node, newPolicyError(ForbiddenStatement, "Only SELECT statements are allowed", nil)
		}
		if n.WithClause != nil {
			ctes := make(map[string]bool, len(v.ctes)+len(n.WithClause.CTEs))
			for name := range v.ctes {
				ctes[name] = true
			}
			for _, cte := range n.WithClause.CTEs {
				ctes[strings.ToLower(sqlast.IdentName(cte.TableName))] = true
			}
			v.ctes = ctes
			// The parser's generic walker does not descend through WithClause
			// or CTE nodes. Inspect their SELECT bodies explicitly.
			for _, cte := range n.WithClause.CTEs {
				if _, err := sqlast.Walk(v, cte.Select); err != nil {
					return nil, node, err
				}
			}
		}
	case *sqlast.QualifiedTableName:
		name := strings.ToLower(sqlast.IdentName(n.Name))
		if n.Schema != nil || (!v.allowed[name] && !v.ctes[name]) {
			return nil, node, newPolicyError(ForbiddenTable, "SQL references a forbidden table", nil)
		}
	case *sqlast.BinaryExpr:
		if n.Op == sqlast.IN || n.Op == sqlast.NOTIN {
			switch rhs := n.Y.(type) {
			case *sqlast.Ident:
				name := strings.ToLower(sqlast.IdentName(rhs))
				if !v.allowed[name] && !v.ctes[name] {
					return nil, node, newPolicyError(ForbiddenTable, "SQL references a forbidden table", nil)
				}
			case *sqlast.QualifiedRef:
				return nil, node, newPolicyError(ForbiddenTable, "SQL references a forbidden table", nil)
			case *sqlast.Call:
				return nil, node, newPolicyError(ForbiddenFunction, "SQL references a forbidden function", nil)
			}
		}
	case *sqlast.QualifiedTableFunctionName:
		// Table-valued functions can disclose metadata and are not physical
		// application tables in the allowlist.
		return nil, node, newPolicyError(ForbiddenFunction, "SQL references a forbidden function", nil)
	case *sqlast.Call:
		name := strings.ToLower(sqlast.IdentName(n.Name))
		if name == "load_extension" || name == "readfile" || name == "writefile" || strings.HasPrefix(name, "pragma_") {
			return nil, node, newPolicyError(ForbiddenFunction, "SQL references a forbidden function", nil)
		}
	}
	return v, node, nil
}

func (v selectPolicyVisitor) VisitEnd(node sqlast.Node) (sqlast.Node, error) { return node, nil }

// Policy remains an unconditionally read-only convenience boundary.
type Policy struct {
	MaxRows    int
	AllowWrite bool
}

func (p Policy) Validate(query string) error {
	_, err := NewSQLiteSelectPolicy(0).Validate(query, emptySchema{})
	return err
}

type emptySchema struct{}

func (emptySchema) TableNames() []string { return nil }

var _ AgentSqlPolicy = SQLiteSelectPolicy{}
