package pglike

import (
	"database/sql"
	"database/sql/driver"
	"net/url"
	"strings"

	_ "modernc.org/sqlite"
)

func init() {
	sql.Register("pglike", &Driver{})
	registerPGFunctions()
}

// Driver wraps the modernc.org/sqlite driver with PostgreSQL SQL translation.
type Driver struct{}

// Open parses the DSN and opens a SQLite connection via the underlying driver.
func (d *Driver) Open(dsn string) (driver.Conn, error) {
	sqliteDSN := parseDSN(dsn)

	// Open via the registered sqlite driver
	db, err := sql.Open("sqlite", sqliteDSN)
	if err != nil {
		return nil, err
	}
	// We need a raw driver.Conn; get one from the underlying sqlite driver
	db.Close()

	// Use the sqlite driver directly
	sqliteDriver := getSQLiteDriver()
	if sqliteDriver == nil {
		return nil, sql.ErrConnDone
	}

	inner, err := sqliteDriver.Open(sqliteDSN)
	if err != nil {
		return nil, err
	}

	return &conn{inner: inner}, nil
}

// getSQLiteDriver retrieves the registered "sqlite" driver.
func getSQLiteDriver() driver.Driver {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil
	}
	defer db.Close()
	return db.Driver()
}

// parseDSN converts various DSN formats to a SQLite-compatible DSN.
func parseDSN(dsn string) string {
	// Already a SQLite DSN
	if dsn == ":memory:" || strings.HasPrefix(dsn, "file:") {
		return dsn
	}

	// PostgreSQL connection URL: postgres://user:pass@host/dbname
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return dsn
		}
		dbname := strings.TrimPrefix(u.Path, "/")
		if dbname == "" {
			dbname = "database"
		}
		return dbname + ".db"
	}

	// PostgreSQL key=value format: host=localhost dbname=myapp
	if strings.Contains(dsn, "=") && !strings.Contains(dsn, "/") {
		parts := strings.Fields(dsn)
		for _, part := range parts {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 && kv[0] == "dbname" {
				return kv[1] + ".db"
			}
		}
		return "database.db"
	}

	// Assume it's a file path
	return dsn
}

// conn wraps a SQLite connection with SQL translation.
type conn struct {
	inner driver.Conn
}

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	translated, err := Translate(query)
	if err != nil {
		return nil, err
	}
	s, err := c.inner.Prepare(translated)
	if err != nil {
		return nil, wrapError(err)
	}
	return &stmt{inner: s}, nil
}

func (c *conn) Close() error {
	return c.inner.Close()
}

func (c *conn) Begin() (driver.Tx, error) {
	t, err := c.inner.Begin() //nolint:staticcheck // implementing deprecated interface
	if err != nil {
		return nil, err
	}
	return &tx{inner: t}, nil
}

// stmt wraps a SQLite prepared statement.
type stmt struct {
	inner driver.Stmt
}

func (s *stmt) Close() error {
	return s.inner.Close()
}

func (s *stmt) NumInput() int {
	return s.inner.NumInput()
}

func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	r, err := s.inner.Exec(args) //nolint:staticcheck // implementing deprecated interface
	if err != nil {
		return nil, wrapError(err)
	}
	return &result{inner: r}, nil
}

func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	r, err := s.inner.Query(args) //nolint:staticcheck // implementing deprecated interface
	if err != nil {
		return nil, wrapError(err)
	}
	return &rows{inner: r}, nil
}

// tx wraps a SQLite transaction.
type tx struct {
	inner driver.Tx
}

func (t *tx) Commit() error {
	return t.inner.Commit()
}

func (t *tx) Rollback() error {
	return t.inner.Rollback()
}

// rows wraps SQLite rows (pass-through).
type rows struct {
	inner driver.Rows
}

func (r *rows) Columns() []string {
	return r.inner.Columns()
}

func (r *rows) Close() error {
	return r.inner.Close()
}

func (r *rows) Next(dest []driver.Value) error {
	return r.inner.Next(dest)
}

// result wraps a SQLite result (pass-through).
type result struct {
	inner driver.Result
}

func (r *result) LastInsertId() (int64, error) {
	return r.inner.LastInsertId()
}

func (r *result) RowsAffected() (int64, error) {
	return r.inner.RowsAffected()
}
