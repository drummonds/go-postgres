# Roadmap

## Phase 2: Extended Compatibility ✓ (v0.2.0)

- [x] `$$dollar-quoted$$` strings
- [x] `generate_series()` via recursive CTE rewriting
- [x] `to_char()` with full PG format string mapping
- [x] Regex operators (`~`, `~*`, `!~`, `!~*`) via custom functions
- [x] `SIMILAR TO` pattern translation
- [x] `NULLS FIRST` / `NULLS LAST` via CASE expression rewriting
- [x] `CREATE SEQUENCE` / `nextval()` / `currval()` emulation with a `_sequences` table
- [x] `INTERVAL` literal parsing and arithmetic
- [x] PG-style error codes in returned errors
- [x] `EXPLAIN` output formatted like PG

## Research: NUMERIC balance views

- [x] **Is an integer+exponent ledger with a NUMERIC(…,7) balance view practical
  on pglike and Postgres?** Build `research/numeric-view/` (own Go module, so
  pgx stays out of the library's go.mod): one program, both drivers through
  `database/sql`, that loads n accounts × 100 movements and times a balance
  view under three storage designs — BIGINT control (no conversion),
  BIGINT at exponent −7 converted in the view with `SUM(amount)::numeric /
  10^7`, and a native `NUMERIC(20,7)` column summed directly. Scale n over
  1k/10k/100k (1M opt-in); measure all-balances and one-account queries,
  load time, and exactness against a Go `math/big` oracle. Backends: pglike
  file, pglike `:memory:`, Postgres (`BENCH_PG_DSN` or a podman
  `postgres:16-alpine`). Output is `research-numeric-views.md` (purpose,
  method, decision table, generated results, verdict), built by
  `docs:build` and linked from `index.md`. Verdict wanted: the conversion
  overhead relative to the control on each backend, and whether native
  NUMERIC storage is even correct on pglike (expected: no — `SUM` over a
  bare NUMERIC column is REAL in SQLite). Feeds go-luca position views
  (gobank ADR-0002 stage 3). Verdict: practical — exact on both drivers, free on PostgreSQL, a few percent on pglike; native NUMERIC storage is inexact on pglike

## Phase 3: Advanced Features

- [x] Catalog views: `information_schema.{tables,columns,table_constraints,key_column_usage,referential_constraints,constraint_column_usage}` and `pg_indexes` installed per-connection so PG-style catalog queries work unchanged
- [x] Schema support by name-mangling in the translator (`crm.customers` → `"crm.customers"`, `public` unprefixed) — one SQLite database, so cross-schema FKs, views, triggers and transactions keep working; `ATTACH DATABASE` rejected (no cross-DB FKs/views, per-connection re-attach, non-atomic WAL commits). `search_path` deferred
- [x] NUMERIC arithmetic: `expr::numeric` / `CAST(... AS NUMERIC)` and `+ - * /`,
  `round()` and comparisons on a numeric operand evaluate as exact decimals
  with PG's scale rules (registered `pg_numeric_*` functions over math/big),
  so a view can publish a NUMERIC column computed from integers and read
  the same on both drivers. Bare NUMERIC columns and decimal literals in
  arithmetic (no cast) and numeric aggregates are a later step — needed by
  go-luca's position views (gobank ADR-0002 stage 3). Verdict: practical — exact on both drivers, free on PostgreSQL, a few percent on pglike; native NUMERIC storage is inexact on pglike
- Array types stored as JSON
- JSONB containment operators (`@>`, `<@`, `#>`)
- `ON CONFLICT ON CONSTRAINT <name>` → resolve to column list (requires schema introspection)
- More comprehensive `ALTER TABLE` support
- `COPY` command support
- `LISTEN` / `NOTIFY` emulation
- Upgrade to `auxten/postgresql-parser` for full AST-based translation
