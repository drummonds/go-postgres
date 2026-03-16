# Changelog

## [Unreleased]

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
