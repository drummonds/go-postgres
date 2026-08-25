package pglike

import (
	"database/sql"
	"sync"
	"testing"
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
