package sql

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
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
	p := NewJSqlParserAgentSqlPolicy(4000)
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
	p := NewJSqlParserAgentSqlPolicy(4000)
	cases := []struct {
		q    string
		code AgentSqlErrorCode
	}{
		{"SELECT * FROM missing", ForbiddenTable},
		{"SELECT readfile('x')", ForbiddenFunction},
		{"SELECT * FROM pragma_table_info('x')", ForbiddenFunction},
		{"WITH recent AS (SELECT * FROM messages) SELECT * FROM recent", ""},
		{"SELECT * FROM information_schema.tables", ForbiddenTable},
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
	got, err := NewJSqlParserAgentSqlPolicy(20).Validate(q, testSchema{})
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
	if _, err := NewJSqlParserAgentSqlPolicy(11).Validate(q, testSchema{}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewJSqlParserAgentSqlPolicy(9).Validate(q, testSchema{}); codeOf(t, err) != SQLTooLong {
		t.Fatalf("%v", err)
	}
}
func TestSQLPolicyErrorSafetyAndCauses(t *testing.T) {
	cause := errors.New("private parser detail")
	wrapped, err := NewAgentSqlPolicyError(MalformedSQL, "SQL could not be parsed", cause)
	if err != nil || !errors.Is(wrapped, cause) {
		t.Fatalf("constructor cause: %v", err)
	}
	p := NewJSqlParserAgentSqlPolicy(4000)
	_, err = p.Validate("SELECT FROM", testSchema{})
	var e *AgentSqlPolicyError
	if !errors.As(err, &e) || e.CodeValue() != MalformedSQL || e.Unwrap() == nil {
		t.Fatalf("%#v", err)
	}
	if strings.Contains(err.Error(), "SELECT FROM") {
		t.Fatal(err)
	}
}
