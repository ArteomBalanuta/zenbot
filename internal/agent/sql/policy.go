package sql

import (
	"errors"
	"strings"
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
