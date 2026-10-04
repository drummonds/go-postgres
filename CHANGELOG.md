# Changelog

## [Unreleased]

### Changed
- README: a pglike file whose views use `::numeric` is readable through
  pglike only. The `pg_numeric_*` functions are registered per connection,
  so another SQLite library (tbls, the `sqlite3` shell) cannot compile those
  views; generate schema documentation from PostgreSQL or through pglike.
- `research-numeric-views.md`: analysis and verdict completed with the
  PostgreSQL 16 run. Integer+exponent storage converted in the view is exact
  on both drivers, free on PostgreSQL and a few percent on pglike; native
  NUMERIC storage is 19–41% slower on PostgreSQL and inexact on pglike.
  Roadmap story ticked.

## [0.7.1] - 2026-10-04

 - Updating numeric research

### Added
- Research study `research-numeric-views.md`: whether a ledger that stores
  BIGINT amounts at exponent −7 can publish NUMERIC(…,7) balances through a
  view at acceptable cost, on pglike and native PostgreSQL, compared with
  storing NUMERIC directly. The program lives in `research/numeric-view/`
  (its own module, so pgx stays out of the library) and is run with
  `task research:numeric-view`; it checks every published balance against a
  `math/big` oracle and writes the tables into the document.

## [0.7.0] - 2026-10-04

 - NUMERIC arithmetic: ::numeric expressions evaluate exactly with PG's scale rules, usable in views

### Added
- NUMERIC arithmetic. An expression with a `::numeric` or
  `CAST(... AS NUMERIC)` operand is evaluated exactly with PG's scale rules
  instead of as integer division, REAL arithmetic or a text comparison:
  `7::numeric / 3` gives `2.3333333333333333`, `'2.50'::numeric * 4` gives
  `10.00`, `'10'::numeric > '9'::numeric` is true. The translator emits
  `pg_numeric`, `pg_numeric_add/sub/mul/div/neg/cmp` and `pg_numeric_round`
  (registered per connection, innocuous so views may use them) and leaves
  every other expression untouched. `x::numeric(p,s)` rounds to `s`.
  Needed so a view can publish a NUMERIC column computed from integer
  columns and read the same on PostgreSQL and pglike. Bare NUMERIC columns
  and decimal literals without a cast are unchanged (documented in the
  README). Corpus: `testdata/corpus/numeric.sql`.

## [0.6.0] - 2026-09-29

 - Adding metadata

### Added
- Schemas: `CREATE SCHEMA [IF NOT EXISTS]`, `DROP SCHEMA [IF EXISTS] ...
  [CASCADE|RESTRICT]` and schema-qualified names (`crm.customers`,
  `crm.customers.id`). A relation in a non-public schema is stored as the
  SQLite table `"crm.customers"` in the same database, so cross-schema joins,
  foreign keys, views and transactions work. Catalog views report real schema
  names, and `information_schema.schemata` is added. `search_path` is not
  supported. (#20)

### Fixed
- `pg_indexes` lists primary-key indexes as `<table>_pkey` with PG's
  `indexdef` shape (`CREATE UNIQUE INDEX t_pkey ON public.t USING btree (id)`),
  including INTEGER PRIMARY KEY tables that have no SQLite index. (#18)
- `information_schema.columns.data_type` / `udt_name` report the PG type as
  declared (`bigint`/`int8`, `timestamp with time zone`/`timestamptz`,
  `character varying`/`varchar`, ...) instead of the SQLite type it was
  translated to. The DDL translator now leaves a `/*pg:<udt>*/` comment after
  each rewritten column type in CREATE/ALTER TABLE; SQLite keeps it in
  `sqlite_master.sql` without affecting column affinity. Tables created by
  earlier versions report the SQLite type mapped to its PG name (`integer`,
  `text`, `real`, `bytea`). (#19)
- `information_schema.columns` includes view columns, as in PostgreSQL.

## [0.5.13] - 2026-09-02

 - Add pg_tables/pg_views catalog views; numbered parameters so a reused $N binds once

### Added
- `pg_tables` and `pg_views` catalog views (bare or `pg_catalog.`-qualified),
  so `SELECT tablename FROM pg_tables WHERE schemaname = 'public'` works
  unchanged. Completes the query set from #17.

### Fixed
- A `$N` parameter reused within one statement (`WHERE a = $1 OR b = $1`)
  now binds once, as in PostgreSQL. Placeholders are translated to SQLite's
  numbered `?N` form instead of positional `?`, which also fixes
  out-of-order parameters (`$2 ... $1`) binding to the wrong argument.

### Changed
- README: the SQLite URI example no longer suggests `_pragma=foreign_keys(1)`,
  which is redundant because `ncruces/go-sqlite3` enables foreign keys by
  default. The `:memory:` pooling section and RESEARCH.md now describe the
  memdb backing introduced in v0.5.11 instead of the removed temp-file and
  single-connection fallbacks. Thanks to @ncruces for the pointers.

## [0.5.12] - 2026-09-02

 - Fix DEFAULT gen_random_uuid() and other function-call defaults in CREATE TABLE

### Fixed
- `DEFAULT gen_random_uuid()` (and any other function call) in a column
  definition no longer fails with `near "(": syntax error`. SQLite only
  accepts a bare literal after `DEFAULT`, so the DDL translator now wraps
  function-call defaults in parentheses, as it already did for `now()`
  and `CURRENT_TIMESTAMP`. New corpus cases cover both
  `DEFAULT gen_random_uuid()` and `TIMESTAMPTZ NOT NULL DEFAULT now()`.
  Closes #9 and #10.

## [0.5.11] - 2026-08-27

 - Upgrade ncruces/go-sqlite3 to v0.35.3; in-memory DSNs now use the memdb VFS on all platforms

### Changed
- Upgraded `ncruces/go-sqlite3` from a March 2026 pre-release pin to
  v0.35.3 (SQLite 3.53.x via the released wasm2go mainline).
- In-memory DSNs are now backed by the pure-Go `memdb` VFS instead of a
  shared temp file: all pool connections share one named in-memory
  database with real locking, identically on native, wasip1 and browser
  WASM. Note memdb allows a single writer at a time (no WAL write
  concurrency); heavily concurrent writers serialise via the busy
  handler.

### Removed
- The single-shared-connection WASM fallback (and its per-open temp-file
  probe) — unreachable now that memdb serves every platform. Its
  query-while-iterating regression test now runs against the public API.

### Fixed
- Under wasip1, upstream v0.35.3's file locking made temp files unusable
  (`disk I/O error`), which silently degraded in-memory databases to the
  shared-connection fallback — losing multi-statement `Exec` and
  `ADD COLUMN IF NOT EXISTS`, and deadlocking when a pool connection
  queried during another's open transaction. The memdb backing removes
  that whole failure mode.

## [0.5.10] - 2026-08-25

### Fixed
- The shared-connection fallback no longer holds its lock until query rows
  are closed — that deadlocked ("all goroutines are asleep") whenever a
  query ran while another query's rows were still open, a normal
  database/sql pattern (hit by the gobank demo's DB explorer in the
  browser). Query results are now materialised in memory and the cursor
  closed before the lock is released; unclosed rows can no longer wedge
  the database.

## [0.5.9] - 2026-08-25

### Fixed
- The single-shared-connection fallback (used in WASM, where no temp file is
  available) now holds its lock for the whole life of a transaction and of
  any open result set instead of per driver call. Previously concurrent pool
  "connections" could interleave statements on the one real SQLite
  connection, corrupting its state — panics inside SQLite ("index out of
  range") that crashed the browser demo. The WASM path is now covered by a
  native test driving the same code.

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
