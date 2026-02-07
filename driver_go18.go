package pglike

import (
	"context"
	"database/sql/driver"
)

// Compile-time interface checks.
var (
	_ driver.Pinger             = (*conn)(nil)
	_ driver.ConnBeginTx        = (*conn)(nil)
	_ driver.ConnPrepareContext = (*conn)(nil)
	_ driver.ExecerContext      = (*conn)(nil)
	_ driver.QueryerContext     = (*conn)(nil)
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
	if preparer, ok := c.inner.(driver.ConnPrepareContext); ok {
		s, err := preparer.PrepareContext(ctx, translated)
		if err != nil {
			return nil, err
		}
		return &stmt{inner: s}, nil
	}
	return c.Prepare(query)
}

// ExecContext implements driver.ExecerContext.
func (c *conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	translated, err := Translate(query)
	if err != nil {
		return nil, err
	}
	if execer, ok := c.inner.(driver.ExecerContext); ok {
		r, err := execer.ExecContext(ctx, translated, args)
		if err != nil {
			return nil, err
		}
		return &result{inner: r}, nil
	}
	// Fallback to Prepare + Exec
	s, err := c.Prepare(translated)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	values := namedToValues(args)
	return s.Exec(values)
}

// QueryContext implements driver.QueryerContext.
func (c *conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	translated, err := Translate(query)
	if err != nil {
		return nil, err
	}
	if queryer, ok := c.inner.(driver.QueryerContext); ok {
		r, err := queryer.QueryContext(ctx, translated, args)
		if err != nil {
			return nil, err
		}
		return &rows{inner: r}, nil
	}
	// Fallback to Prepare + Query
	s, err := c.Prepare(translated)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	values := namedToValues(args)
	return s.Query(values)
}

// ExecContext implements driver.StmtExecContext.
func (s *stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if execer, ok := s.inner.(driver.StmtExecContext); ok {
		r, err := execer.ExecContext(ctx, args)
		if err != nil {
			return nil, err
		}
		return &result{inner: r}, nil
	}
	values := namedToValues(args)
	return s.Exec(values)
}

// QueryContext implements driver.StmtQueryContext.
func (s *stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if queryer, ok := s.inner.(driver.StmtQueryContext); ok {
		r, err := queryer.QueryContext(ctx, args)
		if err != nil {
			return nil, err
		}
		return &rows{inner: r}, nil
	}
	values := namedToValues(args)
	return s.Query(values)
}

// namedToValues converts NamedValue args to positional Value args.
func namedToValues(named []driver.NamedValue) []driver.Value {
	values := make([]driver.Value, len(named))
	for i, nv := range named {
		values[i] = nv.Value
	}
	return values
}
