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

- Schema support via `ATTACH DATABASE`
- Array types stored as JSON
- JSONB containment operators (`@>`, `<@`, `#>`)
- `ON CONFLICT ON CONSTRAINT <name>` → resolve to column list (requires schema introspection)
- More comprehensive `ALTER TABLE` support
- `COPY` command support
- `LISTEN` / `NOTIFY` emulation
- Upgrade to `auxten/postgresql-parser` for full AST-based translation
