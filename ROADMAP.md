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

## Phase 3: Advanced Features

- [x] Catalog views: `information_schema.{tables,columns,table_constraints,key_column_usage,referential_constraints,constraint_column_usage}` and `pg_indexes` installed per-connection so PG-style catalog queries work unchanged
- [x] Schema support by name-mangling in the translator (`crm.customers` → `"crm.customers"`, `public` unprefixed) — one SQLite database, so cross-schema FKs, views, triggers and transactions keep working; `ATTACH DATABASE` rejected (no cross-DB FKs/views, per-connection re-attach, non-atomic WAL commits). `search_path` deferred
- [x] NUMERIC arithmetic: `expr::numeric` / `CAST(... AS NUMERIC)` and `+ - * /`,
  `round()` and comparisons on a numeric operand evaluate as exact decimals
  with PG's scale rules (registered `pg_numeric_*` functions over math/big),
  so a view can publish a NUMERIC column computed from integers and read
  the same on both drivers. Bare NUMERIC columns and decimal literals in
  arithmetic (no cast) and numeric aggregates are a later step — needed by
  go-luca's position views (gobank ADR-0002 stage 3)
- Array types stored as JSON
- JSONB containment operators (`@>`, `<@`, `#>`)
- `ON CONFLICT ON CONSTRAINT <name>` → resolve to column list (requires schema introspection)
- More comprehensive `ALTER TABLE` support
- `COPY` command support
- `LISTEN` / `NOTIFY` emulation
- Upgrade to `auxten/postgresql-parser` for full AST-based translation
