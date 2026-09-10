package sql

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// AgentSqlErrorCode is the closed set of policy and execution error codes.
type AgentSqlErrorCode string

const (
	EmptySQL           AgentSqlErrorCode = "EMPTY_SQL"
	SQLTooLong         AgentSqlErrorCode = "SQL_TOO_LONG"
	MalformedSQL       AgentSqlErrorCode = "MALFORMED_SQL"
	ForbiddenStatement AgentSqlErrorCode = "FORBIDDEN_STATEMENT"
	ForbiddenTable     AgentSqlErrorCode = "FORBIDDEN_TABLE"
	ForbiddenFunction  AgentSqlErrorCode = "FORBIDDEN_FUNCTION"
	Timeout            AgentSqlErrorCode = "TIMEOUT"
	ResultTooLarge     AgentSqlErrorCode = "RESULT_TOO_LARGE"
	ExecutionFailed    AgentSqlErrorCode = "EXECUTION_FAILED"
)

func validCode(c AgentSqlErrorCode) bool {
	switch c {
	case EmptySQL, SQLTooLong, MalformedSQL, ForbiddenStatement, ForbiddenTable, ForbiddenFunction, Timeout, ResultTooLarge, ExecutionFailed:
		return true
	}
	return false
}

// AgentSqlPolicyError is safe for presentation and retains its private cause.
type AgentSqlPolicyError struct {
	Code    AgentSqlErrorCode
	Message string
	Cause   error
}

func (e *AgentSqlPolicyError) Error() string                { return e.Message }
func (e *AgentSqlPolicyError) Unwrap() error                { return e.Cause }
func (e *AgentSqlPolicyError) CodeValue() AgentSqlErrorCode { return e.Code }
func newPolicyError(c AgentSqlErrorCode, msg string, cause error) *AgentSqlPolicyError {
	if !validCode(c) {
		panic("invalid SQL policy code")
	}
	return &AgentSqlPolicyError{Code: c, Message: msg, Cause: cause}
}

// NewAgentSqlPolicyError constructs a policy error and rejects unknown codes.
func NewAgentSqlPolicyError(c AgentSqlErrorCode, message string, cause error) (*AgentSqlPolicyError, error) {
	if !validCode(c) || strings.TrimSpace(message) == "" {
		return nil, errors.New("invalid SQL policy error")
	}
	return newPolicyError(c, message, cause), nil
}

// Schema supplies the physical table allowlist.
type Schema interface{ TableNames() []string }
type AgentSqlPolicy interface {
	Validate(sql string, schema Schema) (ValidatedAgentSql, error)
}

// ValidatedAgentSql is an immutable validated SQL value by construction.
type ValidatedAgentSql struct {
	SQL         string
	Fingerprint string
}

// JSqlParserAgentSqlPolicy validates one read-only SELECT using pg_query.
type JSqlParserAgentSqlPolicy struct{ maxSQLChars int }

func NewJSqlParserAgentSqlPolicy(maxSQLChars int) JSqlParserAgentSqlPolicy {
	return JSqlParserAgentSqlPolicy{maxSQLChars: maxSQLChars}
}
func (p JSqlParserAgentSqlPolicy) Validate(sql string, schema Schema) (ValidatedAgentSql, error) {
	if sql == "" || strings.TrimSpace(sql) == "" {
		return ValidatedAgentSql{}, newPolicyError(EmptySQL, "SQL must not be blank", nil)
	}
	if schema == nil {
		return ValidatedAgentSql{}, newPolicyError(ForbiddenTable, "SQL schema is unavailable", nil)
	}
	if !utf8.ValidString(sql) {
		return ValidatedAgentSql{}, newPolicyError(MalformedSQL, "SQL could not be parsed", errors.New("invalid UTF-8"))
	}
	if p.maxSQLChars > 0 && utf8.RuneCountInString(sql) > p.maxSQLChars {
		return ValidatedAgentSql{}, newPolicyError(SQLTooLong, "SQL exceeds the configured limit", nil)
	}
	parsed, err := pg_query.ParseToJSON(normalizeDialectQuotes(sql))
	if err != nil {
		if isUnsupportedStatement(sql) {
			return ValidatedAgentSql{}, newPolicyError(ForbiddenStatement, "Only SELECT statements are allowed", err)
		}
		return ValidatedAgentSql{}, newPolicyError(MalformedSQL, "SQL could not be parsed", err)
	}
	var root map[string]any
	if json.Unmarshal([]byte(parsed), &root) != nil {
		return ValidatedAgentSql{}, newPolicyError(MalformedSQL, "SQL could not be parsed", errors.New("parser JSON was invalid"))
	}
	stmts, ok := root["stmts"].([]any)
	if !ok || len(stmts) != 1 {
		return ValidatedAgentSql{}, newPolicyError(ForbiddenStatement, "Exactly one SQL statement is allowed", nil)
	}
	st, ok := stmts[0].(map[string]any)
	if !ok {
		return ValidatedAgentSql{}, newPolicyError(ForbiddenStatement, "Only SELECT statements are allowed", nil)
	}
	stmt, ok := st["stmt"].(map[string]any)
	if !ok || len(stmt) != 1 {
		return ValidatedAgentSql{}, newPolicyError(ForbiddenStatement, "Only SELECT statements are allowed", nil)
	}
	if _, ok = stmt["SelectStmt"]; !ok {
		return ValidatedAgentSql{}, newPolicyError(ForbiddenStatement, "Only SELECT statements are allowed", nil)
	}
	allowed := map[string]bool{}
	for _, n := range schema.TableNames() {
		allowed[normalizeIdentifier(n)] = true
	}
	if err := walkPolicy(stmt, allowed, map[string]bool{}); err != nil {
		return ValidatedAgentSql{}, err
	}
	sum := sha256.Sum256([]byte(sql))
	return ValidatedAgentSql{SQL: sql, Fingerprint: hex.EncodeToString(sum[:])}, nil
}

var forbiddenNodes = map[string]bool{"InsertStmt": true, "UpdateStmt": true, "DeleteStmt": true, "CreateStmt": true, "CreateTableAsStmt": true, "DropStmt": true, "AlterTableStmt": true, "TruncateStmt": true, "VacuumStmt": true, "CopyStmt": true, "DoStmt": true, "CallStmt": true, "LockStmt": true, "TransactionStmt": true, "UtilityStmt": true}

func walkPolicy(v any, allowed, ctes map[string]bool) *AgentSqlPolicyError {
	m, isMap := v.(map[string]any)
	if !isMap {
		if a, ok := v.([]any); ok {
			for _, x := range a {
				if e := walkPolicy(x, allowed, ctes); e != nil {
					return e
				}
			}
		}
		return nil
	}
	for k, x := range m {
		if forbiddenNodes[k] {
			return newPolicyError(ForbiddenStatement, "Only read-only SELECT statements are allowed", nil)
		}
		switch k {
		case "SelectStmt":
			if sm, ok := x.(map[string]any); ok {
				if into, exists := sm["intoClause"]; exists && into != nil {
					return newPolicyError(ForbiddenStatement, "Only read-only SELECT statements are allowed", nil)
				}
				if locks, exists := sm["lockingClause"]; exists && locks != nil {
					return newPolicyError(ForbiddenStatement, "Only read-only SELECT statements are allowed", nil)
				}
				if values, exists := sm["valuesLists"]; exists && values != nil {
					return newPolicyError(ForbiddenStatement, "Only SELECT statements are allowed", nil)
				}
				if with, ok := sm["withClause"].(map[string]any); ok {
					if cs, ok := with["ctes"].([]any); ok {
						for _, c := range cs {
							if cm, ok := c.(map[string]any); ok {
								if body, ok := cm["CommonTableExpr"].(map[string]any); ok {
									if n, ok := body["ctename"].(string); ok {
										ctes[normalizeIdentifier(n)] = true
									}
									if q := body["ctequery"]; q == nil {
										return newPolicyError(ForbiddenStatement, "Invalid CTE", nil)
									}
								}
							}
						}
					}
				}
			}
		case "RangeVar":
			r, ok := x.(map[string]any)
			if !ok {
				return newPolicyError(ForbiddenTable, "SQL references a forbidden table", nil)
			}
			name, _ := r["relname"].(string)
			sch, _ := r["schemaname"].(string)
			cat, _ := r["catalogname"].(string)
			n := normalizeIdentifier(name)
			if sch != "" || cat != "" || (!allowed[n] && !ctes[n]) {
				return newPolicyError(ForbiddenTable, "SQL references a forbidden table", nil)
			}
		case "FuncCall":
			fm, ok := x.(map[string]any)
			if !ok {
				return newPolicyError(ForbiddenFunction, "SQL references a forbidden function", nil)
			}
			parts, _ := fm["funcname"].([]any)
			last := ""
			if len(parts) > 0 {
				if z, ok := parts[len(parts)-1].(map[string]any); ok {
					if s, ok := z["String"].(map[string]any); ok {
						last, _ = s["sval"].(string)
					}
				}
			}
			n := normalizeIdentifier(last)
			if n == "load_extension" || n == "readfile" || n == "writefile" || strings.HasPrefix(n, "pragma_") {
				return newPolicyError(ForbiddenFunction, "SQL references a forbidden function", nil)
			}
		}
		if e := walkPolicy(x, allowed, ctes); e != nil {
			return e
		}
	}
	return nil
}
func isUnsupportedStatement(sql string) bool {
	s := strings.TrimSpace(sql)
	end := 0
	for end < len(s) && ((s[end] >= 'a' && s[end] <= 'z') || (s[end] >= 'A' && s[end] <= 'Z') || s[end] == '_') {
		end++
	}
	switch strings.ToLower(s[:end]) {
	case "attach", "detach", "pragma", "vacuum":
		return true
	default:
		return false
	}
}

func normalizeIdentifier(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		a, b := s[0], s[len(s)-1]
		if (a == '"' && b == '"') || (a == '`' && b == '`') || (a == '[' && b == ']') {
			s = s[1 : len(s)-1]
		}
	}
	return strings.ToLower(s)
}
func normalizeDialectQuotes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '`' {
			j := i + 1
			for j < len(s) && s[j] != '`' {
				j++
			}
			if j < len(s) {
				b.WriteByte('"')
				b.WriteString(s[i+1 : j])
				b.WriteByte('"')
				i = j
				continue
			}
		}
		if s[i] == '[' {
			j := i + 1
			for j < len(s) && s[j] != ']' {
				j++
			}
			if j < len(s) {
				b.WriteByte('"')
				b.WriteString(s[i+1 : j])
				b.WriteByte('"')
				i = j
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Policy is retained for source compatibility and remains unconditionally read-only.
type Policy struct {
	MaxRows    int
	AllowWrite bool
}

func (p Policy) Validate(query string) error {
	_, err := NewJSqlParserAgentSqlPolicy(0).Validate(query, compatSchema{})
	return err
}

type compatSchema struct{}

func (compatSchema) TableNames() []string { return nil }

var _ AgentSqlPolicy = JSqlParserAgentSqlPolicy{}
var _ Schema = testSchemaForAssertion{}

type testSchemaForAssertion struct{}

func (testSchemaForAssertion) TableNames() []string { return nil }
