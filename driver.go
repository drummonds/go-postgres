package pglike

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ncruces/go-sqlite3"
	_ "github.com/ncruces/go-sqlite3/driver"
)

func init() {
	sql.Register("pglike", &Driver{})
}

// Compile-time interface check.
var _ driver.DriverContext = (*Driver)(nil)

// Driver wraps the ncruces/go-sqlite3 driver with PostgreSQL SQL translation.
type Driver struct{}

// OpenConnector implements driver.DriverContext.
// For :memory: DSNs, it tries a temp file so pool connections share one database.
// If temp file creation fails (e.g. WASM), it falls back to a single shared
// connection protected by a mutex.
func (d *Driver) OpenConnector(name string) (driver.Connector, error) {
	sqliteDSN := parseDSN(name)
	c := &pglikeConnector{dsn: sqliteDSN, driver: d}

	if name == ":memory:" {
		if tmpDSN, ok := tryTempFile(); ok {
			// Temp file works — all pool connections share this file.
			c.dsn = tmpDSN
			c.tmpFile = tmpDSN
		} else {
			// No usable filesystem (WASM) — single shared connection.
			inner, err := d.openConn(sqliteDSN)
			if err != nil {
				return nil, err
			}
			c.shared = inner
		}
	}

	return c, nil
}

// pglikeConnector implements driver.Connector.
type pglikeConnector struct {
	dsn     string
	tmpFile string      // non-empty when backed by temp file
	shared  driver.Conn // non-nil when using single shared connection (WASM)
	mu      sync.Mutex  // guards shared connection access
	driver  *Driver
}

func (c *pglikeConnector) Connect(_ context.Context) (driver.Conn, error) {
	if c.shared != nil {
		return &sharedConn{real: c.shared, mu: &c.mu}, nil
	}
	return c.driver.openConn(c.dsn)
}

func (c *pglikeConnector) Driver() driver.Driver {
	return c.driver
}

// Close cleans up temp files or the shared connection.
func (c *pglikeConnector) Close() error {
	if c.shared != nil {
		return c.shared.Close()
	}
	if c.tmpFile != "" {
		os.Remove(c.tmpFile + "-wal")
		os.Remove(c.tmpFile + "-shm")
		return os.Remove(c.tmpFile)
	}
	return nil
}

// sharedConn wraps a single real connection with a mutex so the pool
// can hand out multiple "connections" that serialise on one underlying conn.
type sharedConn struct {
	real driver.Conn
	mu   *sync.Mutex
}

func (s *sharedConn) Prepare(query string) (driver.Stmt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.real.Prepare(query)
}

func (s *sharedConn) Begin() (driver.Tx, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.real.Begin() //nolint:staticcheck
}

// Close is a no-op — the real connection is owned by the connector.
func (s *sharedConn) Close() error { return nil }

// tryTempFile creates a temp file and verifies two separate SQLite connections
// can share data through it. ncruces WASM modules have isolated filesystems,
// so this test fails in WASM even though os.CreateTemp succeeds.
// Returns the file path and true on success, or cleans up and returns false.
func tryTempFile() (string, bool) {
	f, err := os.CreateTemp("", "pglike-*.db")
	if err != nil {
		return "", false
	}
	name := f.Name()
	f.Close()

	drv := getSQLiteDriver()
	if drv == nil {
		os.Remove(name)
		return "", false
	}

	// Write a marker table from connection 1.
	c1, err := drv.Open(name)
	if err != nil {
		os.Remove(name)
		return "", false
	}
	s, err := c1.Prepare("CREATE TABLE _pglike_probe (v INTEGER)")
	if err != nil {
		c1.Close()
		os.Remove(name)
		return "", false
	}
	s.Exec(nil) //nolint:staticcheck,errcheck
	s.Close()
	c1.Close()

	// Verify connection 2 can see the table.
	c2, err := drv.Open(name)
	if err != nil {
		os.Remove(name)
		return "", false
	}
	s2, err := c2.Prepare("SELECT v FROM _pglike_probe LIMIT 0")
	if err != nil {
		// Second connection can't see the table — isolated FS.
		c2.Close()
		os.Remove(name)
		return "", false
	}
	s2.Close()

	// Clean up probe table.
	s3, _ := c2.Prepare("DROP TABLE _pglike_probe")
	if s3 != nil {
		s3.Exec(nil) //nolint:staticcheck,errcheck
		s3.Close()
	}
	c2.Close()

	return name, true
}

// Open parses the DSN and opens a SQLite connection via the underlying driver.
func (d *Driver) Open(dsn string) (driver.Conn, error) {
	return d.openConn(parseDSN(dsn))
}

// openConn opens a SQLite connection with the given (already-parsed) DSN.
func (d *Driver) openConn(sqliteDSN string) (driver.Conn, error) {
	sqliteDriver := getSQLiteDriver()
	if sqliteDriver == nil {
		return nil, sql.ErrConnDone
	}

	inner, err := sqliteDriver.Open(sqliteDSN)
	if err != nil {
		return nil, err
	}

	// Register PG-compatible functions on this connection.
	type rawConn interface {
		Raw() *sqlite3.Conn
	}
	if rc, ok := inner.(rawConn); ok {
		if err := registerPGFunctions(rc.Raw()); err != nil {
			inner.Close()
			return nil, err
		}
	}

	c := &conn{inner: inner}

	// Ensure _sequences table exists for sequence emulation.
	_ = c.execDirect("CREATE TABLE IF NOT EXISTS _sequences (name TEXT PRIMARY KEY, current_value INTEGER NOT NULL DEFAULT 0, increment INTEGER NOT NULL DEFAULT 1)")

	return c, nil
}

// getSQLiteDriver retrieves the registered "sqlite3" driver.
func getSQLiteDriver() driver.Driver {
	db, err := sql.Open("sqlite3", ":memory:")
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
func (c *conn) queryDirectInt64(sqlStr string) (int64, error) {
	s, err := c.inner.Prepare(sqlStr)
	if err != nil {
		return 0, err
	}
	defer s.Close()
	r, err := s.Query(nil) //nolint:staticcheck
	if err != nil {
		return 0, err
	}
	defer r.Close()
	_ = r.Columns() // ncruces requires Columns() before Next()
	dest := make([]driver.Value, 1)
	if err := r.Next(dest); err != nil {
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
	if err := r.inner.Next(dest); err != nil {
		return err
	}
	// Coerce string values that look like timestamps to time.Time.
	for i, v := range dest {
		if s, ok := v.(string); ok {
			if t, ok := tryParseTimestamp(s); ok {
				dest[i] = t
			}
		}
	}
	return nil
}

// timestampLayouts lists time formats that SQLite's datetime() function produces.
// Only full datetime formats are included — date-only strings ("2006-01-02")
// are intentionally excluded so that strftime/to_char results remain as strings.
var timestampLayouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02 15:04:05+00:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999Z07:00",
}

// tryParseTimestamp attempts to parse a string as a datetime timestamp.
// Returns the parsed time and true if successful. Only matches full datetime
// strings (date + time), not date-only or time-only strings.
func tryParseTimestamp(s string) (time.Time, bool) {
	// Quick reject: must be long enough for "YYYY-MM-DD HH:MM:SS" and start with date pattern.
	if len(s) < 19 || s[4] != '-' {
		return time.Time{}, false
	}
	for _, layout := range timestampLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
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
