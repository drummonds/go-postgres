// Package harness is what the research studies share: backends, timing
// statistics, duration formatting and splicing results into a document.
package harness

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "git.bytestone.uk/hum3/go-postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// A Backend opens a fresh database. Postgres needs BENCH_PG_DSN; the
// pglike backends use a scratch directory.
type Backend struct {
	Name string
	Open func() (*sql.DB, func(), error)
}

// Backends lists pglike-file, pglike-memory and postgres. Each Open gives a
// fresh database: a new file, a new :memory:, or the same Postgres (the
// caller drops what it creates).
func Backends(tmp string) []Backend {
	return []Backend{
		{Name: "pglike-file", Open: func() (*sql.DB, func(), error) {
			dir, err := os.MkdirTemp(tmp, "pglike")
			if err != nil {
				return nil, nil, err
			}
			path := filepath.Join(dir, "study.db")
			db, err := sql.Open("pglike", path)
			return db, func() { db.Close(); os.RemoveAll(dir) }, err
		}},
		{Name: "pglike-memory", Open: func() (*sql.DB, func(), error) {
			db, err := sql.Open("pglike", ":memory:")
			return db, func() { db.Close() }, err
		}},
		{Name: "postgres", Open: func() (*sql.DB, func(), error) {
			dsn := os.Getenv("BENCH_PG_DSN")
			if dsn == "" {
				return nil, nil, fmt.Errorf("BENCH_PG_DSN not set")
			}
			db, err := sql.Open("pgx", dsn)
			if err != nil {
				return nil, nil, err
			}
			if err := db.Ping(); err != nil {
				db.Close()
				return nil, nil, err
			}
			return db, func() { db.Close() }, nil
		}},
	}
}

// ServerVersion names the engine behind db.
func ServerVersion(ctx context.Context, db *sql.DB) string {
	var v string
	if err := db.QueryRowContext(ctx, "SELECT version()").Scan(&v); err == nil {
		if i := strings.Index(v, " on "); i > 0 {
			v = v[:i]
		}
		return v
	}
	if err := db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&v); err == nil {
		return "pglike (working tree) over SQLite " + v
	}
	return "unknown"
}

// FileSize is the on-disk size of a pglike-file database after a
// checkpoint, or 0 for other backends.
func FileSize(db *sql.DB, backend string) int64 {
	if backend != "pglike-file" {
		return 0
	}
	var path string
	if err := db.QueryRow("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&path); err != nil {
		return 0
	}
	db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}
