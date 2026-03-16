package pglike

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
)

// Compile-time interface checks.
var (
	_ driver.Pinger             = (*conn)(nil)
	_ driver.ConnBeginTx        = (*conn)(nil)
	_ driver.ConnPrepareContext = (*conn)(nil)
	_ driver.ExecerContext      = (*conn)(nil)
	_ driver.StmtExecContext    = (*stmt)(nil)
	_ driver.StmtQueryContext   = (*stmt)(nil)
)

// Ping implements driver.Pinger.
func (c *conn) Ping(ctx context.Context) error {
	if pinger, ok := c.inner.(driver.Pinger); ok {
		return pinger.Ping(ctx)
	}
	return nil
}

// BeginTx implements driver.ConnBeginTx.
func (c *conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if beginner, ok := c.inner.(driver.ConnBeginTx); ok {
		t, err := beginner.BeginTx(ctx, opts)
		if err != nil {
			return nil, err
		}
		return &tx{inner: t}, nil
	}
	return c.Begin()
}

// PrepareContext implements driver.ConnPrepareContext.
func (c *conn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	translated, err := Translate(query)
	if err != nil {
		return nil, err
	}
	translated, err = c.resolveSequenceCalls(translated)
	if err != nil {
		return nil, err
	}
	if preparer, ok := c.inner.(driver.ConnPrepareContext); ok {
		s, err := preparer.PrepareContext(ctx, translated)
		if err != nil {
			return nil, wrapError(err)
		}
		return &stmt{inner: s}, nil
	}
	s, err := c.inner.Prepare(translated)
	if err != nil {
		return nil, wrapError(err)
	}
	return &stmt{inner: s}, nil
}

// ExecContext implements driver.ExecerContext.
func (c *conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	translated, err := Translate(query)
	if err != nil {
		return nil, err
	}
	translated, err = c.resolveSequenceCalls(translated)
	if err != nil {
		return nil, err
	}
	suppressDupCol := isAlterAddColumnIfNotExists(query)

	// Try fast path via inner ExecerContext.
	if execer, ok := c.inner.(driver.ExecerContext); ok {
		r, err := execer.ExecContext(ctx, translated, args)
		if err == nil {
			return &result{inner: r}, nil
		}
		if !errors.Is(err, driver.ErrSkip) {
			if suppressDupCol && isDuplicateColumnError(err) {
				return driver.ResultNoRows, nil
			}
			return nil, wrapError(err)
		}
		// ErrSkip: fall through to prepare+exec
	}

	// Prepare+Exec on inner conn directly (already translated).
	var s driver.Stmt
	if preparer, ok := c.inner.(driver.ConnPrepareContext); ok {
		s, err = preparer.PrepareContext(ctx, translated)
	} else {
		s, err = c.inner.Prepare(translated)
	}
	if err != nil {
		if suppressDupCol && isDuplicateColumnError(err) {
			return driver.ResultNoRows, nil
		}
		return nil, wrapError(err)
	}
	defer s.Close()

	if stmtExecer, ok := s.(driver.StmtExecContext); ok {
		r, err := stmtExecer.ExecContext(ctx, args)
		if err != nil {
			if suppressDupCol && isDuplicateColumnError(err) {
				return driver.ResultNoRows, nil
			}
			return nil, wrapError(err)
		}
		return &result{inner: r}, nil
	}
	r, err := s.Exec(namedToValues(args)) //nolint:staticcheck
	if err != nil {
		if suppressDupCol && isDuplicateColumnError(err) {
			return driver.ResultNoRows, nil
		}
		return nil, wrapError(err)
	}
	return &result{inner: r}, nil
}

// ExecContext implements driver.StmtExecContext.
func (s *stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if execer, ok := s.inner.(driver.StmtExecContext); ok {
		r, err := execer.ExecContext(ctx, args)
		if err != nil {
			return nil, wrapError(err)
		}
		return &result{inner: r}, nil
	}
	values := namedToValues(args)
	return s.Exec(values) //nolint:staticcheck
}

// QueryContext implements driver.StmtQueryContext.
func (s *stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if queryer, ok := s.inner.(driver.StmtQueryContext); ok {
		r, err := queryer.QueryContext(ctx, args)
		if err != nil {
			return nil, wrapError(err)
		}
		return &rows{inner: r}, nil
	}
	values := namedToValues(args)
	return s.Query(values) //nolint:staticcheck
}

// namedToValues converts NamedValue args to positional Value args.
func namedToValues(named []driver.NamedValue) []driver.Value {
	values := make([]driver.Value, len(named))
	for i, nv := range named {
		values[i] = nv.Value
	}
	return values
}

// isAlterAddColumnIfNotExists checks if a query is an ALTER TABLE ADD COLUMN IF NOT EXISTS.
func isAlterAddColumnIfNotExists(query string) bool {
	upper := strings.ToUpper(query)
	return strings.Contains(upper, "ALTER") &&
		strings.Contains(upper, "ADD") &&
		strings.Contains(upper, "IF NOT EXISTS")
}

// isDuplicateColumnError checks if an error is a SQLite "duplicate column name" error.
func isDuplicateColumnError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column name")
}
