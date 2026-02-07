package pglike

import "strings"

// PGError represents a PostgreSQL-compatible error with an error code.
type PGError struct {
	Code    string // 5-char SQLSTATE code (e.g. "23505")
	Message string // human-readable error message
	inner   error  // underlying SQLite error
}

func (e *PGError) Error() string {
	return e.Message
}

func (e *PGError) Unwrap() error {
	return e.inner
}

// SQLState returns the 5-character SQLSTATE error code.
func (e *PGError) SQLState() string {
	return e.Code
}

// wrapError wraps a SQLite error with a PG-compatible error code.
// Returns the original error if it's nil or can't be classified.
func wrapError(err error) error {
	if err == nil {
		return nil
	}

	msg := err.Error()
	code := classifySQLiteError(msg)

	return &PGError{
		Code:    code,
		Message: msg,
		inner:   err,
	}
}

// classifySQLiteError maps a SQLite error message to a PG SQLSTATE code.
func classifySQLiteError(msg string) string {
	lower := strings.ToLower(msg)

	switch {
	case strings.Contains(lower, "unique constraint") || strings.Contains(lower, "unique_constraint"):
		return "23505" // unique_violation
	case strings.Contains(lower, "not null constraint") || strings.Contains(lower, "not_null_constraint"):
		return "23502" // not_null_violation
	case strings.Contains(lower, "foreign key constraint") || strings.Contains(lower, "foreign_key_constraint"):
		return "23503" // foreign_key_violation
	case strings.Contains(lower, "check constraint") || strings.Contains(lower, "check_constraint"):
		return "23514" // check_violation
	case strings.Contains(lower, "no such table") || strings.Contains(lower, "no_such_table"):
		return "42P01" // undefined_table
	case strings.Contains(lower, "no such column") || strings.Contains(lower, "no_such_column"):
		return "42703" // undefined_column
	case strings.Contains(lower, "syntax error"):
		return "42601" // syntax_error
	default:
		return "XX000" // internal_error
	}
}
