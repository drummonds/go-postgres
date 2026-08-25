# Changelog

## [Unreleased]

## [0.5.8] - 2026-08-25

### Fixed
- Concurrent write transactions on in-memory DSNs no longer fail with
  "database is locked" (SQLITE_BUSY): the shared temp file is now opened
  with `_txlock=immediate`, WAL journal mode and an explicit busy timeout.
  Previously deferred read-then-write transactions on the default rollback
  journal hit SQLite's deadlock-avoidance path, which returns SQLITE_BUSY
  without consulting the busy handler.

## [0.5.7] - 2026-08-25

### Fixed
- All in-memory DSN spellings (`file::memory:`, `file:...?mode=memory`, with
  or without query parameters) now share one database across pool
  connections, matching the existing `:memory:` behaviour. Previously each
  pool connection to `file::memory:` got its own private empty database,
  surfacing as "no such table" under concurrent load.

## [0.5.6] - 2026-08-11

 - Docs and link cleanup after the forge migration

### Fixed
- "Source" links relabelled Codeberg → Forgejo; "Mirror (GitHub)" links now point at https://github.com/drummonds/go-postgres instead of the old Codeberg URL.
- Documentation links moved off retired statichost to https://go-postgres.docs.bytestone.uk/.

### Changed
- Docs deploy switched from statichost to rsync (`tp pages deploy`).

## [0.5.5] - 2026-08-08

 - Migrate forge references from codeberg.org to git.bytestone.uk

### Changed
- Module path and self-referencing URLs now point at the new Forgejo instance following the move off Codeberg.

## [0.5.4] - 2026-05-10

 - Adding catalog coverage for lofidb

### Added
- PG-compatible catalog views installed on every connection: `information_schema.tables`, `information_schema.columns`, `information_schema.table_constraints`, `information_schema.key_column_usage`, `information_schema.referential_constraints`, `information_schema.constraint_column_usage`, and `pg_indexes`. Plus a pglike-only helper view `pg_index_columns` exposing index columns without a `pg_index`/`pg_class`/`pg_attribute` join.
- `current_schema()` returns `'public'` and `current_database()` returns `'main'` so PG-style catalog filters work unchanged.
- Translator rewrites `information_schema.X`, `pg_catalog.X`, bare `pg_indexes`, and bare `pg_index_columns` to mangled view names (`_pglike_<schema>_<view>`) so the same SQL runs on pglike and real Postgres.

## [0.5.3] - 2026-03-24

 - Fix soak metrics and add chart generation

## [0.5.2] - 2026-03-23

 - Working on bench testing

## [0.5.1] - 2026-03-23

 - fixing soak:cloud and mod path

## [0.5.0] - 2026-03-19

 - moving from wazero to wasm2go

## [0.4.3] - 2026-03-18

 - making splits safe

## [0.4.2] - 2026-03-18

 - fixing lint isue

### Fixed
- `:memory:` connection pooling now works under WASM (wasip1) — falls back to single shared connection when temp files can't be shared across ncruces module instances

### Added
- WASM cross-compilation tests (wasm_test.go)

## [0.4.1] - 2026-03-18

 - fix linting

## [0.4.0] - 2026-03-16

 - switching to ncruces/sqlite

## [0.3.3] - 2026-03-15

## [0.3.2] - 2026-03-15

 - Combining docs

## [0.3.1] - 2026-03-07

 - Switching to shopspring decimal for numeric

## [0.3.0] - 2026-02-08

### Added
- ALTER TABLE ADD COLUMN IF NOT EXISTS support
- Tests verifying INSERT RETURNING works via SQLite 3.35+

### Fixed
- NULLS FIRST/LAST for table-qualified and expression columns
- Coerce SQLite timestamp strings to time.Time on Scan
- DEFAULT CURRENT_TIMESTAMP not wrapped in parentheses for SQLite
- SERIAL PRIMARY KEY generating duplicate PRIMARY KEY in SQLite

## [0.2.0] - 2026-02-07

### Added
- Dollar-quoted string support (`$$...$$`, `$tag$...$tag$`)
- `generate_series()` via recursive CTE rewriting
- `to_char()` full format mapping with runtime fallback
- Regex operator support (`~`, `~*`, `!~`, `!~*`)
- `SIMILAR TO` pattern matching support
- `NULLS FIRST` / `NULLS LAST` ordering support
- `CREATE SEQUENCE` / `nextval()` / `currval()` emulation
- `INTERVAL` literal parsing and datetime arithmetic
- PG-compatible error codes wrapping SQLite errors
- `EXPLAIN` output translation

## [0.1.0] - 2026-02-07

### Added
- Initial pglike driver: PG-compatible SQL over SQLite
- DDL type mappings (SERIAL, BOOLEAN, VARCHAR, TIMESTAMP, etc.)
- Expression translations (::cast, ILIKE, TRUE/FALSE, E'strings')
- Function translations (NOW, date_trunc, EXTRACT, left/right, concat)
- Custom SQLite functions (gen_random_uuid, md5, split_part, pg_typeof)
- DSN parsing (PostgreSQL URLs, key=value, SQLite paths)
