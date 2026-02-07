# Roadmap

## Phase 2: Extended Compatibility

- `$$dollar-quoted$$` strings
- `generate_series()` via recursive CTE rewriting
- `to_char()` with full PG format string mapping
- Regex operators (`~`, `~*`, `!~`, `!~*`) via custom functions
- `SIMILAR TO` pattern translation
- `NULLS FIRST` / `NULLS LAST` via CASE expression rewriting
- `CREATE SEQUENCE` / `nextval()` / `currval()` emulation with a `_sequences` table
- `INTERVAL` literal parsing and arithmetic
- PG-style error codes in returned errors
- `EXPLAIN` output formatted like PG

## Phase 3: Advanced Features

- Schema support via `ATTACH DATABASE`
- Array types stored as JSON
- JSONB containment operators (`@>`, `<@`, `#>`)
- More comprehensive `ALTER TABLE` support
- `COPY` command support
- `LISTEN` / `NOTIFY` emulation
- Upgrade to `auxten/postgresql-parser` for full AST-based translation
