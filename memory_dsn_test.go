package pglike

import (
	"database/sql"
	"sync"
	"testing"
	"time"
)

func TestIsMemoryDSN(t *testing.T) {
	cases := []struct {
		dsn  string
		want bool
	}{
		{":memory:", true},
		{"file::memory:", true},
		{"file::memory:?_pragma=temp_store(2)", true},
		{"file::memory:?cache=shared", true},
		{"file:test.db?mode=memory", true},
		{"file:test.db", false},
		{"file:test.db?_pragma=temp_store(2)", false},
		{"test.db", false},
		{"file:", true}, // empty path is in-memory per SQLite URI rules
	}
	for _, c := range cases {
		if got := isMemoryDSN(c.dsn); got != c.want {
			t.Errorf("isMemoryDSN(%q) = %v, want %v", c.dsn, got, c.want)
		}
	}
}

// TestMemoryDSNConcurrentWriteTransactions verifies concurrent read-then-write
// transactions on an in-memory DSN don't fail with "database is locked".
// Regression: the shared temp file was opened with default (deferred)
// transactions on a rollback journal, where SQLite returns SQLITE_BUSY
// immediately — bypassing the busy handler — when a deferred transaction
// upgrades to a write lock while another connection holds one. Surfaced as
// persistCustomer/ledger "sqlite3: database is locked" in the gobank demo's
// native run.
func TestMemoryDSNConcurrentWriteTransactions(t *testing.T) {
	db, err := sql.Open("pglike", "file::memory:?_pragma=temp_store(2)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(4)

	if _, err := db.Exec(`CREATE TABLE counters (id INTEGER PRIMARY KEY, n INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO counters (id, n) VALUES (1, 0)`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	const workers = 8
	const txPerWorker = 25
	var wg sync.WaitGroup
	errs := make(chan error, workers*txPerWorker)
	for range workers {
		wg.Go(func() {
			for range txPerWorker {
				tx, err := db.Begin()
				if err != nil {
					errs <- err
					continue
				}
				var n int
				if err := tx.QueryRow(`SELECT n FROM counters WHERE id = 1`).Scan(&n); err != nil {
					errs <- err
					_ = tx.Rollback()
					continue
				}
				if _, err := tx.Exec(`UPDATE counters SET n = $1 WHERE id = 1`, n+1); err != nil {
					errs <- err
					_ = tx.Rollback()
					continue
				}
				if err := tx.Commit(); err != nil {
					errs <- err
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent tx: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT n FROM counters WHERE id = 1`).Scan(&n); err != nil {
		t.Fatalf("final read: %v", err)
	}
	if n != workers*txPerWorker {
		t.Errorf("n = %d, want %d (lost updates)", n, workers*txPerWorker)
	}
}

// openSharedConnDB builds a DB on the single-shared-connection fallback the
// driver uses when no temp file is usable (the WASM path), so that code path
// is testable natively.
func openSharedConnDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	d := &Driver{}
	inner, err := d.openConn(parseDSN(dsn))
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(&pglikeConnector{dsn: parseDSN(dsn), driver: d, shared: inner})
	t.Cleanup(func() { db.Close() })
	return db
}

// TestSharedConnConcurrentTransactions exercises the WASM single-shared-
// connection fallback under concurrent transactions and overlapping reads.
// Regression: the mutex was only held per Prepare/Begin call, so two pool
// "connections" could interleave statements on the one real SQLite
// connection — a second BEGIN inside an open transaction, statements joining
// a foreign transaction — crashing the gobank demo in the browser.
func TestSharedConnConcurrentTransactions(t *testing.T) {
	db := openSharedConnDB(t, "file::memory:?_pragma=temp_store(2)")
	db.SetMaxOpenConns(4)

	if _, err := db.Exec(`CREATE TABLE counters (id INTEGER PRIMARY KEY, n INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO counters (id, n) VALUES (1, 0)`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	const workers = 8
	const txPerWorker = 25
	var wg sync.WaitGroup
	errs := make(chan error, 2*workers*txPerWorker)
	for range workers {
		wg.Go(func() {
			for range txPerWorker {
				tx, err := db.Begin()
				if err != nil {
					errs <- err
					continue
				}
				var n int
				if err := tx.QueryRow(`SELECT n FROM counters WHERE id = 1`).Scan(&n); err != nil {
					errs <- err
					_ = tx.Rollback()
					continue
				}
				if _, err := tx.Exec(`UPDATE counters SET n = $1 WHERE id = 1`, n+1); err != nil {
					errs <- err
					_ = tx.Rollback()
					continue
				}
				if err := tx.Commit(); err != nil {
					errs <- err
				}
			}
		})
		// A polling reader alongside each writer, like the demo dashboard.
		wg.Go(func() {
			for range txPerWorker {
				var n int
				if err := db.QueryRow(`SELECT n FROM counters WHERE id = 1`).Scan(&n); err != nil {
					errs <- err
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent op: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT n FROM counters WHERE id = 1`).Scan(&n); err != nil {
		t.Fatalf("final read: %v", err)
	}
	if n != workers*txPerWorker {
		t.Errorf("n = %d, want %d (lost updates)", n, workers*txPerWorker)
	}
}

// TestSharedConnQueryWhileIterating verifies queries issued while another
// query's rows are still open don't deadlock on the shared-connection
// fallback. Regression: the lock was held until rows were closed, so this
// completely normal database/sql pattern self-deadlocked ("all goroutines
// are asleep" in the browser demo's DB explorer).
func TestSharedConnQueryWhileIterating(t *testing.T) {
	db := openSharedConnDB(t, "file::memory:")
	db.SetMaxOpenConns(4)

	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	for i := 1; i <= 5; i++ {
		if _, err := db.Exec(`INSERT INTO items (id, name) VALUES ($1, $2)`, i, "n"); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	done := make(chan error, 1)
	go func() {
		rows, err := db.Query(`SELECT id FROM items ORDER BY id`)
		if err != nil {
			done <- err
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				done <- err
				return
			}
			// Nested query while the outer rows are open.
			var name string
			if err := db.QueryRow(`SELECT name FROM items WHERE id = $1`, id).Scan(&name); err != nil {
				done <- err
				return
			}
		}
		done <- rows.Err()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("query while iterating: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("deadlock: nested query never completed")
	}

	// Rows left unclosed must not wedge the database either.
	leaked, err := db.Query(`SELECT id FROM items`)
	if err != nil {
		t.Fatal(err)
	}
	_ = leaked // deliberately not closed until test end
	defer leaked.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&n); err != nil {
		t.Fatalf("query after leaked rows: %v", err)
	}
	if n != 5 {
		t.Errorf("count = %d, want 5", n)
	}
}

// TestMemoryDSNVariantsSharedAcrossConnections verifies every in-memory DSN
// spelling shares one database across pool connections. Regression: only the
// literal ":memory:" was special-cased, so "file::memory:?..." gave each pool
// connection its own private empty database — surfacing as "no such table"
// under concurrent load (the gobank demo's add-customers crash).
func TestMemoryDSNVariantsSharedAcrossConnections(t *testing.T) {
	for _, dsn := range []string{
		":memory:",
		"file::memory:",
		"file::memory:?_pragma=temp_store(2)",
	} {
		t.Run(dsn, func(t *testing.T) {
			db, err := sql.Open("pglike", dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.SetMaxOpenConns(4)

			if _, err := db.Exec(`CREATE TABLE t (id TEXT PRIMARY KEY, name TEXT)`); err != nil {
				t.Fatalf("create table: %v", err)
			}
			if _, err := db.Exec(`INSERT INTO t (id, name) VALUES ('a1', 'Alice')`); err != nil {
				t.Fatalf("insert: %v", err)
			}

			// Pin one connection so concurrent readers must open another.
			conn, err := db.Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()

			var wg sync.WaitGroup
			errs := make(chan error, 10)
			for range 10 {
				wg.Go(func() {
					var name string
					if err := db.QueryRow(`SELECT name FROM t WHERE id = 'a1'`).Scan(&name); err != nil {
						errs <- err
					}
				})
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Errorf("concurrent read: %v", err)
			}
		})
	}
}
