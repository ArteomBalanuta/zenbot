package sql

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

type testSchema []string

func (s testSchema) TableNames() []string { return append([]string(nil), s...) }

func codeOf(t *testing.T, err error) AgentSqlErrorCode {
	t.Helper()
	var e *AgentSqlPolicyError
	if !errors.As(err, &e) {
		t.Fatalf("error type %T: %v", err, err)
	}
	return e.CodeValue()
}

func TestSQLPolicyRejectsBlankAndWrites(t *testing.T) {
	p := NewSQLiteSelectPolicy(4000)
	for _, q := range []string{"", " /* comment */ ", "INSERT INTO messages VALUES (1)", "SELECT 1; SELECT 2", "VALUES (1)", "SELECT * FROM messages FOR UPDATE"} {
		if _, err := p.Validate(q, testSchema{"messages"}); err == nil {
			t.Errorf("accepted %q", q)
		}
	}
	_, err := p.Validate("", testSchema{})
	if got := codeOf(t, err); got != EmptySQL {
		t.Fatal(got)
	}
}

func TestSQLPolicySchemaFunctionsAndCTE(t *testing.T) {
	p := NewSQLiteSelectPolicy(4000)
	cases := []struct {
		q    string
		code AgentSqlErrorCode
	}{
		{"SELECT * FROM missing", ForbiddenTable},
		{"SELECT readfile('x')", ForbiddenFunction},
		{"SELECT * FROM pragma_table_info('x')", ForbiddenFunction},
		{"WITH recent AS (SELECT * FROM messages) SELECT * FROM recent", ""},
		{"SELECT * FROM information_schema.tables", ForbiddenTable},
		{"WITH x AS (SELECT * FROM sqlite_schema) SELECT * FROM x", ForbiddenTable},
		{"WITH x AS (SELECT readfile('secret')) SELECT * FROM x", ForbiddenFunction},
		{"SELECT 1 IN secret", ForbiddenTable},
		{"SELECT 1 NOT IN main.secret", ForbiddenTable},
		{"SELECT 1 IN pragma_table_info('messages')", ForbiddenFunction},
		{"SELECT (SELECT name FROM sqlite_schema LIMIT 1)", ForbiddenTable},
		{"SELECT 1 WHERE 1 IN (SELECT rootpage FROM sqlite_schema)", ForbiddenTable},
		{"SELECT (SELECT readfile('secret'))", ForbiddenFunction},
		{"SELECT (SELECT name FROM sqlite_schema LIMIT 1) ISNULL", ForbiddenTable},
		{"SELECT readfile('secret') NOTNULL", ForbiddenFunction},
		{"SELECT readfile('secret') NOT NULL", ForbiddenFunction},
	}
	for _, tc := range cases {
		_, err := p.Validate(tc.q, testSchema{"messages"})
		if tc.code == "" && err != nil {
			t.Errorf("%q: %v", tc.q, err)
		}
		if tc.code != "" && codeOf(t, err) != tc.code {
			t.Errorf("%q: got %v", tc.q, codeOf(t, err))
		}
	}
}
func TestSQLPolicyPreservesInputAndHashesOriginal(t *testing.T) {
	q := "  SELECT 1  "
	got, err := NewSQLiteSelectPolicy(20).Validate(q, testSchema{})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(q))
	if got.SQL != q || got.Fingerprint != hex.EncodeToString(sum[:]) || strings.ToLower(got.Fingerprint) != got.Fingerprint {
		t.Fatalf("%+v", got)
	}
}
func TestSQLPolicyUnicodeLength(t *testing.T) {
	q := "SELECT '😀'"
	if _, err := NewSQLiteSelectPolicy(11).Validate(q, testSchema{}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSQLiteSelectPolicy(9).Validate(q, testSchema{}); codeOf(t, err) != SQLTooLong {
		t.Fatalf("%v", err)
	}
}

func TestSQLPolicySQLiteDialect(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE messages(id INTEGER PRIMARY KEY, name TEXT, trip TEXT, created_on INTEGER); INSERT INTO messages VALUES(1,'alice',NULL,0)"); err != nil {
		t.Fatal(err)
	}
	p := NewSQLiteSelectPolicy(4000)
	for _, query := range []string{
		"SELECT name FROM messages WHERE name GLOB 'a*'",
		"SELECT name FROM messages WHERE trip ISNULL",
		"SELECT name FROM messages WHERE id == 1",
		"SELECT strftime('%Y', created_on / 1000, 'unixepoch') FROM messages",
		"WITH recent AS (SELECT name FROM messages) SELECT * FROM recent",
		"SELECT (SELECT name FROM messages LIMIT 1)",
	} {
		if _, err := p.Validate(query, testSchema{"messages"}); err != nil {
			t.Errorf("SQLite query %q rejected: %v", query, err)
		}
		rows, err := db.Query(query)
		if err != nil {
			t.Fatalf("validated query does not execute: %v", err)
		}
		if !rows.Next() {
			t.Errorf("query returned no row: %q (%v)", query, rows.Err())
		}
		rows.Close()
	}
}
func TestSQLPolicyErrorSafetyAndCauses(t *testing.T) {
	cause := errors.New("private parser detail")
	wrapped, err := NewAgentSqlPolicyError(MalformedSQL, "SQL could not be parsed", cause)
	if err != nil || !errors.Is(wrapped, cause) {
		t.Fatalf("constructor cause: %v", err)
	}
	p := NewSQLiteSelectPolicy(4000)
	_, err = p.Validate("SELECT FROM", testSchema{})
	var e *AgentSqlPolicyError
	if !errors.As(err, &e) || e.CodeValue() != MalformedSQL || e.Unwrap() == nil {
		t.Fatalf("%#v", err)
	}
	if strings.Contains(err.Error(), "SELECT FROM") {
		t.Fatal(err)
	}
}
