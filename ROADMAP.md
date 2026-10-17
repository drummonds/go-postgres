# Roadmap

## Exact NUMERIC

NUMERIC values behave as exact decimals on pglike everywhere PostgreSQL treats them so, not only behind a cast or an aggregate.
- #24 Bare NUMERIC columns compare, order and do arithmetic exactly

## PostgreSQL types

PG types beyond SQLite's storage classes, stored so the same SQL reads the same on both drivers.
- #25 Array types stored as JSON
- #26 JSONB containment operators (@>, <@, #>)

## Statements

PG statements and clauses pglike does not yet translate.
- #27 ON CONFLICT ON CONSTRAINT <name> resolves to its column list
- #28 More comprehensive ALTER TABLE support
- #29 COPY command support
- #30 LISTEN / NOTIFY emulation

## Translator

How the translator understands SQL.
- #31 AST-based translation via auxten/postgresql-parser

## Testing and stability

Proving pglike matches PostgreSQL and stays up under load.
- #5 Dual-target TestCompat: run the same SQL against pglike and real PostgreSQL
- #6 Stability/soak testing infrastructure
- #7 Determine browser WASM memory limits and stabilise the in-memory database
