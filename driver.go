package pglike

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
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

	// Ensure _sequences table exists for sequence emulation
	if execer, ok := inner.(interface {
		Exec(query string, args []driver.Value) (driver.Result, error)
	}); ok {
		execer.Exec("CREATE TABLE IF NOT EXISTS _sequences (name TEXT PRIMARY KEY, current_value INTEGER NOT NULL DEFAULT 0, increment INTEGER NOT NULL DEFAULT 1)", nil) //nolint:errcheck
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

// execDirect executes a SQL statement directly on the inner connection without translation.
func (c *conn) execDirect(sql string) error {
	s, err := c.inner.Prepare(sql)
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = s.Exec(nil) //nolint:staticcheck
	return err
}

// queryDirectInt64 executes a query and returns a single int64 value.
func (c *conn) queryDirectInt64(sql string) (int64, error) {
	s, err := c.inner.Prepare(sql)
	if err != nil {
		return 0, err
	}
	defer s.Close()
	rows, err := s.Query(nil) //nolint:staticcheck
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	dest := make([]driver.Value, 1)
	if err := rows.Next(dest); err != nil {
		return 0, err
	}
	if v, ok := dest[0].(int64); ok {
		return v, nil
	}
	return 0, fmt.Errorf("unexpected type from sequence query")
}

// nextval increments and returns the next value for a sequence.
func (c *conn) nextval(seqName string) (int64, error) {
	sql := fmt.Sprintf("UPDATE _sequences SET current_value = current_value + increment WHERE name = '%s'", seqName)
	if err := c.execDirect(sql); err != nil {
		return 0, err
	}
	return c.queryDirectInt64(fmt.Sprintf("SELECT current_value FROM _sequences WHERE name = '%s'", seqName))
}

// currval returns the current value of a sequence.
func (c *conn) currval(seqName string) (int64, error) {
	return c.queryDirectInt64(fmt.Sprintf("SELECT current_value FROM _sequences WHERE name = '%s'", seqName))
}

// resolveSequenceCalls replaces nextval('name') and currval('name') with their values.
func (c *conn) resolveSequenceCalls(query string) (string, error) {
	for {
		idx := strings.Index(query, "nextval(")
		if idx == -1 {
			break
		}
		seqName, end, ok := extractSeqName(query, idx+len("nextval("))
		if !ok {
			break
		}
		val, err := c.nextval(seqName)
		if err != nil {
			return "", wrapError(err)
		}
		query = query[:idx] + fmt.Sprintf("%d", val) + query[end:]
	}
	for {
		idx := strings.Index(query, "currval(")
		if idx == -1 {
			break
		}
		seqName, end, ok := extractSeqName(query, idx+len("currval("))
		if !ok {
			break
		}
		val, err := c.currval(seqName)
		if err != nil {
			return "", wrapError(err)
		}
		query = query[:idx] + fmt.Sprintf("%d", val) + query[end:]
	}
	return query, nil
}

// extractSeqName extracts a sequence name from 'name') starting at pos.
// Returns the name, end position (after closing paren), and success.
func extractSeqName(s string, pos int) (string, int, bool) {
	if pos >= len(s) || s[pos] != '\'' {
		return "", 0, false
	}
	end := strings.Index(s[pos+1:], "'")
	if end == -1 {
		return "", 0, false
	}
	name := s[pos+1 : pos+1+end]
	closePos := pos + 1 + end + 1 // after closing quote
	if closePos >= len(s) || s[closePos] != ')' {
		return "", 0, false
	}
	return name, closePos + 1, true
}

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	translated, err := Translate(query)
	if err != nil {
		return nil, err
	}
	translated, err = c.resolveSequenceCalls(translated)
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
