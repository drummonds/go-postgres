package pglike

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestDriverCreateTableAndInsert(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`CREATE TABLE users (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		active BOOLEAN DEFAULT TRUE
	)`)
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	_, err = db.Exec("INSERT INTO users (name) VALUES (?)", "Alice")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	var id int64
	var name string
	var active int64
	err = db.QueryRow("SELECT id, name, active FROM users WHERE name = ?", "Alice").Scan(&id, &name, &active)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}

	if id != 1 {
		t.Errorf("id = %d, want 1", id)
	}
	if name != "Alice" {
		t.Errorf("name = %q, want Alice", name)
	}
	if active != 1 {
		t.Errorf("active = %d, want 1", active)
	}
}

func TestDriverNow(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`CREATE TABLE events (
		id SERIAL PRIMARY KEY,
		created_at TIMESTAMP DEFAULT (datetime('now'))
	)`)
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	_, err = db.Exec("INSERT INTO events (id) VALUES (1)")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	var createdAt string
	err = db.QueryRow("SELECT created_at FROM events WHERE id = 1").Scan(&createdAt)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}

	if createdAt == "" {
		t.Error("created_at is empty, expected a timestamp")
	}
	// Should look like "2024-01-15 10:30:00" or similar
	if len(createdAt) < 10 {
		t.Errorf("created_at = %q, doesn't look like a timestamp", createdAt)
	}
}

func TestDriverTypeCast(t *testing.T) {
	db := openTestDB(t)

	var val int64
	err := db.QueryRow("SELECT CAST('42' AS INTEGER)").Scan(&val)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if val != 42 {
		t.Errorf("got %d, want 42", val)
	}
}

func TestDriverBooleans(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`CREATE TABLE flags (
		id INTEGER PRIMARY KEY,
		enabled INTEGER DEFAULT 1
	)`)
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	_, err = db.Exec("INSERT INTO flags (id, enabled) VALUES (1, 1)")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT count(*) FROM flags WHERE enabled = 1").Scan(&count)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestDriverTransaction(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	_, err = tx.Exec("INSERT INTO t (id, val) VALUES (1, 'hello')")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT count(*) FROM t").Scan(&count)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d after rollback, want 0", count)
	}
}

func TestDriverGenRandomUUID(t *testing.T) {
	db := openTestDB(t)

	var uuid string
	err := db.QueryRow("SELECT gen_random_uuid()").Scan(&uuid)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}

	// UUID should be 36 chars: 8-4-4-4-12
	if len(uuid) != 36 {
		t.Errorf("uuid length = %d, want 36: %q", len(uuid), uuid)
	}
	if uuid[8] != '-' || uuid[13] != '-' || uuid[18] != '-' || uuid[23] != '-' {
		t.Errorf("uuid format invalid: %q", uuid)
	}
}

func TestDriverMD5(t *testing.T) {
	db := openTestDB(t)

	var hash string
	err := db.QueryRow("SELECT md5('hello')").Scan(&hash)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}

	// md5("hello") = "5d41402abc4b2a76b9719d911017c592"
	if hash != "5d41402abc4b2a76b9719d911017c592" {
		t.Errorf("md5('hello') = %q, want 5d41402abc4b2a76b9719d911017c592", hash)
	}
}

func TestDriverSplitPart(t *testing.T) {
	db := openTestDB(t)

	var part string
	err := db.QueryRow("SELECT split_part('a,b,c', ',', 2)").Scan(&part)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}

	if part != "b" {
		t.Errorf("split_part = %q, want b", part)
	}
}

func TestDriverPgTypeof(t *testing.T) {
	db := openTestDB(t)

	var typ string
	err := db.QueryRow("SELECT pg_typeof(42)").Scan(&typ)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}

	if typ != "integer" {
		t.Errorf("pg_typeof(42) = %q, want integer", typ)
	}
}

func TestDriverMultipleRows(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	for i, name := range []string{"alpha", "beta", "gamma"} {
		_, err := db.Exec("INSERT INTO items (id, name) VALUES (?, ?)", i+1, name)
		if err != nil {
			t.Fatalf("INSERT %d: %v", i, err)
		}
	}

	rows, err := db.Query("SELECT name FROM items ORDER BY id")
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}

	if len(names) != 3 {
		t.Fatalf("got %d rows, want 3", len(names))
	}
	if names[0] != "alpha" || names[1] != "beta" || names[2] != "gamma" {
		t.Errorf("names = %v, want [alpha beta gamma]", names)
	}
}

func TestDriverComplexTable(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		email VARCHAR(255) UNIQUE,
		active BOOLEAN DEFAULT TRUE,
		created_at TIMESTAMP DEFAULT (datetime('now'))
	)`)
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	_, err = db.Exec("INSERT INTO users (name, email) VALUES (?, ?)", "Alice", "alice@example.com")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	_, err = db.Exec("INSERT INTO users (name, email) VALUES (?, ?)", "Bob", "bob@example.com")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT count(*) FROM users WHERE active = 1").Scan(&count)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestDriverDollarQuotedStrings(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	// Insert using dollar-quoted string
	_, err = db.Exec("INSERT INTO t (id, val) VALUES (1, $$hello world$$)")
	if err != nil {
		t.Fatalf("INSERT with $$: %v", err)
	}

	var val string
	err = db.QueryRow("SELECT val FROM t WHERE id = 1").Scan(&val)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if val != "hello world" {
		t.Errorf("val = %q, want 'hello world'", val)
	}

	// Insert with single quotes inside dollar-quoted string
	_, err = db.Exec("INSERT INTO t (id, val) VALUES (2, $$it's a test$$)")
	if err != nil {
		t.Fatalf("INSERT with quotes: %v", err)
	}

	err = db.QueryRow("SELECT val FROM t WHERE id = 2").Scan(&val)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if val != "it's a test" {
		t.Errorf("val = %q, want \"it's a test\"", val)
	}
}

func TestPGErrorUniqueViolation(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT UNIQUE)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	_, err = db.Exec("INSERT INTO t (id, name) VALUES (1, 'alice')")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	// Duplicate unique value should produce a PGError
	_, err = db.Exec("INSERT INTO t (id, name) VALUES (2, 'alice')")
	if err == nil {
		t.Fatal("expected error on duplicate unique, got nil")
	}

	var pgErr *PGError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected PGError, got %T: %v", err, err)
	}
	if pgErr.Code != "23505" {
		t.Errorf("error code = %q, want 23505 (unique_violation)", pgErr.Code)
	}
}

func TestPGErrorNotNullViolation(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT NOT NULL)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	_, err = db.Exec("INSERT INTO t (id, name) VALUES (1, NULL)")
	if err == nil {
		t.Fatal("expected error on NOT NULL violation, got nil")
	}

	var pgErr *PGError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected PGError, got %T: %v", err, err)
	}
	if pgErr.Code != "23502" {
		t.Errorf("error code = %q, want 23502 (not_null_violation)", pgErr.Code)
	}
}

func TestPGErrorUndefinedTable(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("SELECT * FROM nonexistent_table")
	if err == nil {
		t.Fatal("expected error on missing table, got nil")
	}

	var pgErr *PGError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected PGError, got %T: %v", err, err)
	}
	if pgErr.Code != "42P01" {
		t.Errorf("error code = %q, want 42P01 (undefined_table)", pgErr.Code)
	}
}

func TestPGErrorSQLState(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("INSERT INTO nonexistent (id) VALUES (1)")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var pgErr *PGError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected PGError, got %T: %v", err, err)
	}
	// Should have a non-empty code
	if pgErr.SQLState() == "" {
		t.Error("SQLState() returned empty string")
	}
	// Error() should include the message
	if pgErr.Error() == "" {
		t.Error("Error() returned empty string")
	}
}

func TestDriverRegexOperators(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	_, err = db.Exec("INSERT INTO t VALUES (1, 'Alice'), (2, 'Bob'), (3, 'alex')")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	// ~ case sensitive: should match 'Alice' only (starts with A)
	var count int
	err = db.QueryRow("SELECT count(*) FROM t WHERE name ~ '^A'").Scan(&count)
	if err != nil {
		t.Fatalf("~ query: %v", err)
	}
	if count != 1 {
		t.Errorf("~ '^A' count = %d, want 1", count)
	}

	// ~* case insensitive: should match 'Alice' and 'alex'
	err = db.QueryRow("SELECT count(*) FROM t WHERE name ~* '^a'").Scan(&count)
	if err != nil {
		t.Fatalf("~* query: %v", err)
	}
	if count != 2 {
		t.Errorf("~* '^a' count = %d, want 2", count)
	}

	// !~ negated case sensitive: should match 'Bob' and 'alex'
	err = db.QueryRow("SELECT count(*) FROM t WHERE name !~ '^A'").Scan(&count)
	if err != nil {
		t.Fatalf("!~ query: %v", err)
	}
	if count != 2 {
		t.Errorf("!~ '^A' count = %d, want 2", count)
	}

	// !~* negated case insensitive: should match 'Bob' only
	err = db.QueryRow("SELECT count(*) FROM t WHERE name !~* '^a'").Scan(&count)
	if err != nil {
		t.Fatalf("!~* query: %v", err)
	}
	if count != 1 {
		t.Errorf("!~* '^a' count = %d, want 1", count)
	}
}

func TestDriverSequences(t *testing.T) {
	db := openTestDB(t)

	// Create the _sequences table (driver should auto-create it)
	_, err := db.Exec("CREATE SEQUENCE test_seq")
	if err != nil {
		t.Fatalf("CREATE SEQUENCE: %v", err)
	}

	// nextval should return incrementing values
	var val1, val2 int64
	err = db.QueryRow("SELECT nextval('test_seq')").Scan(&val1)
	if err != nil {
		t.Fatalf("nextval 1: %v", err)
	}
	err = db.QueryRow("SELECT nextval('test_seq')").Scan(&val2)
	if err != nil {
		t.Fatalf("nextval 2: %v", err)
	}

	if val1 != 1 {
		t.Errorf("nextval 1 = %d, want 1", val1)
	}
	if val2 != 2 {
		t.Errorf("nextval 2 = %d, want 2", val2)
	}

	// currval should return last value
	var curr int64
	err = db.QueryRow("SELECT currval('test_seq')").Scan(&curr)
	if err != nil {
		t.Fatalf("currval: %v", err)
	}
	if curr != 2 {
		t.Errorf("currval = %d, want 2", curr)
	}
}

func TestDriverGenerateSeries(t *testing.T) {
	db := openTestDB(t)

	rows, err := db.Query("SELECT * FROM generate_series(1, 5)")
	if err != nil {
		t.Fatalf("generate_series: %v", err)
	}
	defer rows.Close()

	var vals []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		vals = append(vals, v)
	}
	if len(vals) != 5 {
		t.Fatalf("got %d rows, want 5: %v", len(vals), vals)
	}
	for i, v := range vals {
		if v != int64(i+1) {
			t.Errorf("row %d = %d, want %d", i, v, i+1)
		}
	}
}

func TestDriverInterval(t *testing.T) {
	db := openTestDB(t)

	// INTERVAL addition
	var result time.Time
	err := db.QueryRow("SELECT '2024-01-15 10:00:00' + INTERVAL '1 day'").Scan(&result)
	if err != nil {
		t.Fatalf("INTERVAL +: %v", err)
	}
	expected := time.Date(2024, 1, 16, 10, 0, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("+ INTERVAL '1 day' = %v, want %v", result, expected)
	}

	// INTERVAL subtraction
	err = db.QueryRow("SELECT '2024-01-15 10:00:00' - INTERVAL '2 hours'").Scan(&result)
	if err != nil {
		t.Fatalf("INTERVAL -: %v", err)
	}
	expected = time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("- INTERVAL '2 hours' = %v, want %v", result, expected)
	}
}

func TestDriverToChar(t *testing.T) {
	db := openTestDB(t)

	// Test strftime fast path
	var result string
	err := db.QueryRow("SELECT to_char('2024-03-15 14:30:00', 'YYYY-MM-DD')").Scan(&result)
	if err != nil {
		t.Fatalf("to_char fast path: %v", err)
	}
	if result != "2024-03-15" {
		t.Errorf("to_char YYYY-MM-DD = %q, want '2024-03-15'", result)
	}

	// Test runtime path with month name
	err = db.QueryRow("SELECT pg_to_char('2024-03-15 14:30:00', 'Mon DD, YYYY')").Scan(&result)
	if err != nil {
		t.Fatalf("pg_to_char: %v", err)
	}
	if result != "Mar 15, 2024" {
		t.Errorf("pg_to_char Mon DD, YYYY = %q, want 'Mar 15, 2024'", result)
	}
}

func TestDriverNullsOrdering(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("CREATE TABLE t2 (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	_, err = db.Exec("INSERT INTO t2 VALUES (1, 'a'), (2, NULL), (3, 'c')")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	// NULLS FIRST: NULL should come first
	rows, err := db.Query("SELECT val FROM t2 ORDER BY val ASC NULLS FIRST")
	if err != nil {
		t.Fatalf("NULLS FIRST query: %v", err)
	}
	defer rows.Close()

	var vals []sql.NullString
	for rows.Next() {
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		vals = append(vals, v)
	}
	if len(vals) != 3 {
		t.Fatalf("got %d rows, want 3", len(vals))
	}
	if vals[0].Valid {
		t.Errorf("first row should be NULL, got %q", vals[0].String)
	}
}

func TestDriverSimilarTo(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	_, err = db.Exec("INSERT INTO t VALUES (1, 'foo'), (2, 'bar'), (3, 'baz'), (4, 'qux')")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	// SIMILAR TO with alternation
	var count int
	err = db.QueryRow("SELECT count(*) FROM t WHERE name SIMILAR TO '%(foo|bar)%'").Scan(&count)
	if err != nil {
		t.Fatalf("SIMILAR TO: %v", err)
	}
	if count != 2 {
		t.Errorf("SIMILAR TO count = %d, want 2", count)
	}

	// NOT SIMILAR TO
	err = db.QueryRow("SELECT count(*) FROM t WHERE name NOT SIMILAR TO '%(foo|bar)%'").Scan(&count)
	if err != nil {
		t.Fatalf("NOT SIMILAR TO: %v", err)
	}
	if count != 2 {
		t.Errorf("NOT SIMILAR TO count = %d, want 2", count)
	}
}

func TestDriverExplain(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("CREATE TABLE t3 (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	// EXPLAIN should return rows (query plan output)
	rows, err := db.Query("EXPLAIN SELECT * FROM t3 WHERE id = 1")
	if err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	defer rows.Close()

	var rowCount int
	for rows.Next() {
		rowCount++
		cols, _ := rows.Columns()
		// Just verify we can scan the columns
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("Scan: %v", err)
		}
	}
	if rowCount == 0 {
		t.Error("EXPLAIN returned no rows")
	}
}

func TestDriverTimestampScan(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`CREATE TABLE ts_test (
		id SERIAL PRIMARY KEY,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	_, err = db.Exec("INSERT INTO ts_test (id) VALUES (1)")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	// Scanning into *time.Time should work without custom scanner
	var createdAt time.Time
	err = db.QueryRow("SELECT created_at FROM ts_test WHERE id = 1").Scan(&createdAt)
	if err != nil {
		t.Fatalf("Scan into time.Time: %v", err)
	}

	if createdAt.IsZero() {
		t.Error("created_at is zero, expected a valid timestamp")
	}

	// Also test with an explicit datetime value
	_, err = db.Exec("INSERT INTO ts_test (id, created_at) VALUES (2, '2024-03-15 14:30:00')")
	if err != nil {
		t.Fatalf("INSERT explicit: %v", err)
	}

	var ts2 time.Time
	err = db.QueryRow("SELECT created_at FROM ts_test WHERE id = 2").Scan(&ts2)
	if err != nil {
		t.Fatalf("Scan explicit timestamp: %v", err)
	}

	if ts2.Year() != 2024 || ts2.Month() != 3 || ts2.Day() != 15 {
		t.Errorf("expected 2024-03-15, got %v", ts2)
	}
	if ts2.Hour() != 14 || ts2.Minute() != 30 {
		t.Errorf("expected 14:30, got %v", ts2)
	}
}

func TestDriverTimestampFormats(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("CREATE TABLE ts_fmt (id INTEGER PRIMARY KEY, ts TIMESTAMP)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	// Test various timestamp formats that SQLite might produce
	formats := []struct {
		id    int
		value string
		year  int
		month time.Month
		day   int
	}{
		{1, "2024-06-01 12:00:00", 2024, time.June, 1},
		{2, "2024-03-15T14:30:00Z", 2024, time.March, 15},
		{3, "2024-12-25 08:00:00+00:00", 2024, time.December, 25},
	}

	for _, f := range formats {
		_, err := db.Exec("INSERT INTO ts_fmt VALUES (?, ?)", f.id, f.value)
		if err != nil {
			t.Fatalf("INSERT id=%d: %v", f.id, err)
		}
	}

	for _, f := range formats {
		var ts time.Time
		err := db.QueryRow("SELECT ts FROM ts_fmt WHERE id = ?", f.id).Scan(&ts)
		if err != nil {
			t.Fatalf("Scan id=%d (%s): %v", f.id, f.value, err)
		}
		if ts.Year() != f.year || ts.Month() != f.month || ts.Day() != f.day {
			t.Errorf("id=%d: expected %d-%02d-%02d, got %v", f.id, f.year, f.month, f.day, ts)
		}
	}
}

func TestDriverGroupConcat(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("CREATE TABLE t (id INTEGER, val TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	_, err = db.Exec("INSERT INTO t VALUES (1, 'a'), (2, 'b'), (3, 'c')")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	var agg string
	err = db.QueryRow("SELECT group_concat(val, ', ') FROM t ORDER BY id").Scan(&agg)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	// group_concat doesn't guarantee order without subquery, but we test it works
	if !strings.Contains(agg, "a") || !strings.Contains(agg, "b") || !strings.Contains(agg, "c") {
		t.Errorf("group_concat = %q, expected a, b, c", agg)
	}
}
