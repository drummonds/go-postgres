# go-postgres Deep Research

> **Note**: This project has no notification system or task scheduling flow.
> It is a PostgreSQL-to-SQLite SQL translation driver. This document covers
> the actual architecture in depth.

## What It Is

A pure Go `database/sql` driver registered as `"pglike"` that accepts PostgreSQL SQL,
translates it to SQLite-compatible SQL at runtime, and executes against `ncruces/go-sqlite3`
(SQLite compiled to WASM, run via wazero — no CGo). ~5300 lines of Go.

Module: `github.com/drummonds/go-postgres`

## Architecture Overview

```
Application → database/sql("pglike") → driver.go → Translate() → SQLite
                                          ↓
                                    registerPGFunctions()
                                    (gen_random_uuid, md5, etc.)
```

The core design is **token-based multi-pass translation**:

1. Tokenize PG SQL into typed tokens
2. Run ordered translation passes (each mutates the token slice)
3. Reassemble tokens back to SQL string
4. Execute against SQLite

---

## Tokenizer (translate.go)

11 token types: `TokKeyword`, `TokIdent`, `TokString`, `TokNumber`, `TokOperator`,
`TokParam`, `TokParen`, `TokComma`, `TokSemicolon`, `TokDot`, `TokWhitespace`, `TokComment`.

Notable parsing:
- **Dollar-quoted strings**: `$$body$$` and `$tag$body$tag$` — converted to standard `'body'`
- **Escape strings**: `E'\n\t'` recognized as single token
- **Regex operators**: `~`, `~*`, `!~`, `!~*` (multi-char operator lookahead)
- **Parameters**: `$1` distinguished from `$$` dollar-quoting
- **80+ SQL keywords** normalized to uppercase for matching

Whitespace and comments are preserved as tokens so reassembly maintains formatting.

---

## Translation Pipeline (translate.go:Translate)

**Order is critical** — each pass assumes prior passes are complete.

```go
tokens = translateExplain(tokens)        // EXPLAIN ANALYZE → EXPLAIN QUERY PLAN
tokens = translateGenerateSeries(tokens) // generate_series → recursive CTE
tokens = translateSequenceDDL(tokens)    // CREATE/DROP SEQUENCE → _sequences table DML
tokens = translateInterval(tokens)       // expr ± INTERVAL → datetime(expr, modifier)
tokens = translateDDL(tokens)            // Type mapping, SERIAL, DEFAULT NOW()
tokens = translateExpressions(tokens)    // Casts, ILIKE, booleans, regex, SIMILAR TO
tokens = translateFunctions(tokens)      // NOW(), date_trunc, EXTRACT, left/right, concat, aggs
tokens = translateNullsOrdering(tokens)  // NULLS FIRST/LAST → CASE WHEN IS NULL
tokens = translateParams(tokens)         // $1,$2 → ?,?
```

### Why this order matters

1. `translateExpressions` contains `translateIsTrueFalse` which must run **before**
   `translateBooleans` (otherwise `TRUE`/`FALSE` are already `1`/`0` and `IS TRUE` won't match).
   Both are inside `translateExpressions` in the correct sub-order.

2. DDL runs before expressions to avoid mangling type-cast syntax in column definitions.

3. Params converted last — `$1` could be confused with `$$` dollar-quoting if converted early.

4. `generate_series` runs early because it restructures the entire query (prepends a CTE).

---

## DDL Translation (translate_ddl.go)

### Type Mapping

| PostgreSQL | SQLite |
|---|---|
| SERIAL/BIGSERIAL/SMALLSERIAL | INTEGER PRIMARY KEY AUTOINCREMENT |
| BOOLEAN/BOOL | INTEGER |
| VARCHAR/CHARACTER VARYING/TEXT | TEXT |
| TIMESTAMP/TIMESTAMPTZ | TEXT |
| DATE/TIME/TIMETZ | TEXT |
| UUID | TEXT |
| BYTEA | BLOB |
| JSON/JSONB | TEXT |
| SMALLINT/INT2/BIGINT/INT8 | INTEGER |
| FLOAT4/REAL/DOUBLE PRECISION | REAL |
| NUMERIC/DECIMAL | TEXT |

### SERIAL handling

`translateSerial()` converts `SERIAL` to `INTEGER PRIMARY KEY AUTOINCREMENT` and strips
any redundant `PRIMARY KEY` constraint that appears later in the column definition via
`stripPrimaryKey()`.

### DEFAULT functions

- `DEFAULT NOW()` → `DEFAULT (datetime('now'))`
- `DEFAULT CURRENT_TIMESTAMP` → `DEFAULT (datetime('now'))`
- `DEFAULT CURRENT_DATE` → `DEFAULT (date('now'))`

SQLite requires function calls in DEFAULT to be wrapped in parentheses.

### ALTER TABLE ADD COLUMN IF NOT EXISTS

SQLite doesn't support `IF NOT EXISTS` on `ADD COLUMN`. The driver strips the clause
at translation time, and at execution time (driver_go18.go) suppresses duplicate-column
errors by checking `isDuplicateColumnError()`.

---

## Expression Translation (translate_expr.go)

### Type casts: `expr::type` → `CAST(expr AS type)`

`extractLeftExpr()` walks backwards to find the LHS expression, handling:
- Simple identifiers: `col`
- Table-qualified: `t.col`
- Parenthesized groups: `(expr)`
- Function calls: `func(args)`

**Slice aliasing protection**: extracted tokens are always `copy()`ed before the
source slice is truncated. This is a known bug vector — if you skip the copy,
`append()` on the truncated slice overwrites the extracted data.

```go
exprTokens := make([]Token, len(exprRef))
copy(exprTokens, exprRef)  // MUST copy before truncating
out = out[:len(out)-len(exprTokens)]
```

### Boolean handling

`translateIsTrueFalse()` runs first:
- `IS TRUE` → `= 1`, `IS FALSE` → `= 0`
- `IS NOT TRUE` → `!= 1`, `IS NOT FALSE` → `!= 0`

Then `translateBooleans()`:
- `TRUE` → `1`, `FALSE` → `0`

### Regex operators

- `expr ~ pat` → `pg_regex_match(expr, pat, 0)`
- `expr ~* pat` → `pg_regex_match(expr, pat, 1)` (case-insensitive)
- `expr !~ pat` → `NOT pg_regex_match(expr, pat, 0)`

### SIMILAR TO

- `expr SIMILAR TO pat` → `pg_similar_match(expr, pat)`
- Handles optional `NOT` before SIMILAR

### ILIKE

- `ILIKE` → `LIKE` (SQLite LIKE is already case-insensitive for ASCII)

### Escape strings

`E'\n\t'` → resolves `\n`, `\t`, `\r`, `\\`, `\'` to actual characters.

---

## Function Translation (translate_func.go)

| PostgreSQL | SQLite |
|---|---|
| `NOW()` | `datetime('now')` |
| `CURRENT_DATE/TIME/TIMESTAMP` | `date/time/datetime('now')` |
| `date_trunc('day', expr)` | `date(expr)` |
| `date_trunc('hour', expr)` | `strftime('%Y-%m-%d %H:00:00', expr)` |
| `EXTRACT(year FROM expr)` | `CAST(strftime('%Y', expr) AS INTEGER)` |
| `left(s, n)` | `substr(s, 1, n)` |
| `right(s, n)` | `substr(s, -n)` |
| `concat(a, b)` | `(COALESCE(a,'') \|\| COALESCE(b,''))` |
| `string_agg(expr, sep)` | `group_concat(expr, sep)` |
| `array_agg(expr)` | `json_group_array(expr)` |
| `date_part('field', expr)` | `CAST(strftime(fmt, expr) AS INTEGER)` |
| `to_char(expr, fmt)` | strftime mapping or runtime `pg_to_char()` |

### LEFT/RIGHT ambiguity

`LEFT` and `RIGHT` are both SQL keywords (for JOINs) and PG function names.
`translateLeftRight()` checks for `TokIdent` OR `TokKeyword`, then requires
`(` to follow — disambiguating function calls from join syntax.

### `parseFuncArgs()` helper

Safely splits function arguments by tracking parenthesis depth. Returns `[][]Token`
for each argument. Used by date_trunc, EXTRACT, left/right, concat, aggregates.

---

## Sequence Emulation (translate_sequence.go + driver.go)

Uses a `_sequences` table created on every connection:

```sql
CREATE TABLE IF NOT EXISTS _sequences (
  name TEXT PRIMARY KEY,
  current_value INTEGER NOT NULL DEFAULT 0,
  increment INTEGER NOT NULL DEFAULT 1
)
```

- `CREATE SEQUENCE name` → `INSERT OR IGNORE INTO _sequences ...`
- `DROP SEQUENCE name` → `DELETE FROM _sequences WHERE name = ...`
- `nextval('name')` → resolved at prepare time by updating `current_value += increment`
  and substituting the literal value into the query
- `currval('name')` → resolved at prepare time by reading `current_value`

The resolution is **string-based** (`resolveSequenceCalls()` scans the translated SQL
for `nextval(` / `currval(` patterns), not token-based.

---

## generate_series (translate_genseries.go)

Converts `FROM generate_series(start, stop[, step])` into a recursive CTE:

```sql
-- Input:
SELECT * FROM generate_series(1, 10, 2) AS t

-- Output:
WITH RECURSIVE _gs(value) AS (
  SELECT 1 UNION ALL SELECT value + 2 FROM _gs WHERE value + 2 <= 10
) SELECT * FROM _gs AS t
```

Prepends CTE, rewrites FROM clause, preserves alias.

---

## INTERVAL Arithmetic (translate_interval.go)

```sql
-- Input:
created_at + INTERVAL '1 day'

-- Output:
datetime(created_at, '+1 day')
```

Supports both `INTERVAL '1 day'` and `INTERVAL '1' DAY` syntax.
Units: year(s), month(s), day(s), hour(s), minute(s), second(s).

---

## NULLS FIRST/LAST (translate_order.go)

```sql
-- Input:
ORDER BY name ASC NULLS FIRST

-- Output:
ORDER BY (CASE WHEN name IS NULL THEN 0 ELSE 1 END), name ASC
```

Handles simple columns, table-qualified (`t.col`), and expression columns (`LOWER(name)`).
Uses `findColumnExprStart()` to walk backwards and find expression boundaries,
including through parenthesized function calls.

---

## Driver Layer (driver.go, driver_go18.go)

### Connection Pooling (`:memory:`)

`database/sql` pools connections. For `:memory:`, each `Open()` call creates an isolated
database. The driver implements `driver.DriverContext.OpenConnector()` to handle this:

1. **Probe**: `tryTempFile()` creates a temp file, writes from connection 1, reads from
   connection 2. If both work, the temp file is used as the shared backing store.
2. **Fallback**: If the probe fails (WASM — ncruces modules have isolated filesystems),
   the connector keeps a single real connection and hands out `sharedConn` wrappers that
   serialize access via mutex.

The `pglikeConnector` implements `io.Closer` — `db.Close()` cleans up temp files or the
shared connection.

### DSN Parsing

- SQLite paths: `myapp.db`, `file:...`, `:memory:` → pass through
- PostgreSQL URLs: `postgres://user@host/dbname` → `dbname.db`
- PostgreSQL key=value: `dbname=myapp` → `myapp.db`
- Fallback: `database.db`

### Query Flow

`Prepare()` / `PrepareContext()`:
1. `Translate(sql)` — token-based PG→SQLite
2. `resolveSequenceCalls(translated)` — string-based nextval/currval resolution
3. `innerConn.Prepare(resolved)` — SQLite preparation

### Result Coercion

`rows.Next()` scans values and attempts to parse SQLite text timestamps into `time.Time`
using `tryParseTimestamp()` with 6 layout variants. Only converts strings containing
both date and time components.

### Error Wrapping (pgerror.go)

SQLite errors are wrapped with PostgreSQL SQLSTATE codes:
- `"unique constraint"` → `23505` (unique_violation)
- `"not null constraint"` → `23502` (not_null_violation)
- `"foreign key constraint"` → `23503` (foreign_key_violation)
- `"no such table"` → `42P01` (undefined_table)
- Default → `XX000` (internal_error)

---

## Custom SQLite Functions (pgfuncs.go)

Registered via `ncruces/go-sqlite3` scalar function API on each new connection.

| Function | Behavior |
|---|---|
| `gen_random_uuid()` | Random UUID v4 string |
| `md5(text)` | Hex MD5 hash |
| `split_part(str, delim, field)` | 1-indexed field extraction |
| `pg_regex_match(str, pattern, ci)` | POSIX regex match, returns 0/1 |
| `pg_to_char(datetime, fmt)` | PG format string → formatted datetime |
| `pg_similar_match(str, pattern)` | SQL SIMILAR TO matching |
| `pg_typeof(expr)` | Returns type name string |

---

## Known Bugs / Edge Cases / Risks

### 1. Slice Aliasing (Defended)

Throughout translate_expr.go, translate_order.go, translate_interval.go — any time a
sub-slice is extracted from the token array and the original is truncated, `copy()` is
used to prevent data corruption. This was a real bug that was fixed. Any new translation
function must follow this pattern.

### 2. Sequence Resolution is String-Based

`resolveSequenceCalls()` scans the SQL string for `nextval('` and `currval('` patterns.
This means:
- Sequences inside string literals could be falsely matched
- Sequence names with special characters could break parsing
- Each `nextval()` call actually increments the sequence during prepare, not execute

### 3. DSN Parsing Fallback

`postgres://localhost/` (no dbname) falls through to `database.db`. Not a bug per se
but could surprise users.

### 4. No Query Caching

Full tokenization + multi-pass translation runs on every `Prepare()` call. Fine for
embedded/testing use, not suitable for high-throughput OLTP.

### 5. generate_series Limitations

Only handles `FROM generate_series(...)` — not subqueries or joins with generate_series.
The CTE prepend restructures the whole query.

### 6. WASM `:memory:` Concurrency

Under WASM (`wasip1`), `:memory:` databases use a single shared connection with mutex
serialization. This means all pool connections are serialized — fine for testing, but
not suitable for concurrent workloads. File-based DSNs are unaffected (each connection
opens the file independently).

### 7. SIMILAR TO Regex Conversion

`convertSimilarToRegex()` does basic `%`→`.*` and `_`→`.` conversion but doesn't handle
all SQL SIMILAR TO edge cases (character classes with special escaping, etc.).

---

## Test Coverage

**Well covered**: DDL translations, type mappings, expression casts, boolean handling,
basic CRUD, transactions, UUID/MD5/split_part functions, INSERT RETURNING.

**Gaps**: generate_series integration, sequence lifecycle, INTERVAL arithmetic,
NULLS FIRST/LAST, regex operators, SIMILAR TO, error code classification,
malformed SQL inputs, concurrent sequence access.

---

## File Map

| File | Lines | Purpose |
|---|---|---|
| translate.go | 478 | Tokenizer + pipeline orchestration |
| translate_ddl.go | 425 | Type mapping, SERIAL, DEFAULT functions |
| translate_expr.go | 491 | Casts, regex, booleans, ILIKE, escape strings |
| translate_func.go | 512 | Date/time/string function translation |
| translate_genseries.go | 136 | generate_series → recursive CTE |
| translate_interval.go | 96 | INTERVAL arithmetic |
| translate_sequence.go | 151 | CREATE/DROP SEQUENCE |
| translate_order.go | 155 | NULLS FIRST/LAST |
| driver.go | 495 | Driver, connector, DSN, connection pooling, sequences |
| driver_go18.go | 172 | Context-aware methods |
| pgfuncs.go | 265 | Custom SQLite function registration |
| pgerror.go | 64 | PG SQLSTATE error wrapping |
| driver_test.go | 990 | Integration tests |
| translate_test.go | 810 | Translation unit tests |
| wasm_test.go | 74 | WASM cross-compilation tests |
| example/main.go | 73 | Usage example |
