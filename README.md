# go-postgres

A lightweight, pure Go `database/sql` driver that accepts PostgreSQL SQL syntax but executes against SQLite under the hood via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite). This lets Go applications written for PostgreSQL run against a local SQLite file -- ideal for testing, embedded use, CLI tools, and development environments. Files remain SQLite-compatible.

The driver registers as `"pglike"` to avoid conflicts with existing PG drivers (`lib/pq`, `pgx`).

## Installation

```bash
go get github.com/drummonds/go-postgres
```

## Quick Start

```go
package main

import (
    "database/sql"
    "fmt"
    _ "github.com/drummonds/go-postgres"
)

func main() {
    db, _ := sql.Open("pglike", "example.db")
    defer db.Close()

    db.Exec(`CREATE TABLE IF NOT EXISTS users (
        id SERIAL PRIMARY KEY,
        name VARCHAR(100) NOT NULL,
        email VARCHAR(255) UNIQUE,
        active BOOLEAN DEFAULT TRUE,
        created_at TIMESTAMP DEFAULT NOW()
    )`)

    db.Exec("INSERT INTO users (name, email) VALUES ($1, $2)", "Alice", "alice@example.com")

    rows, _ := db.Query("SELECT id, name, active FROM users WHERE active = TRUE")
    defer rows.Close()
    for rows.Next() {
        var id int64
        var name string
        var active int64
        rows.Scan(&id, &name, &active)
        fmt.Printf("id=%d name=%s active=%d\n", id, name, active)
    }
}
```

## Architecture

```
User Go Code
    |  sql.Open("pglike", "myapp.db")
    v
database/sql
    |
    v
go-postgres driver (this project)
    |  1. Translate PG SQL -> SQLite SQL
    |  2. Register PG-compatible functions
    |  3. Delegate to SQLite engine
    v
modernc.org/sqlite (pure Go SQLite engine)
    |
    v
SQLite database file
```

## DSN Formats

The driver accepts several DSN formats:

| Format | Example | Behaviour |
|--------|---------|-----------|
| SQLite file path | `myapp.db` | Opens the file directly |
| SQLite URI | `file:myapp.db?_pragma=foreign_keys(1)` | Passed through to SQLite |
| In-memory | `:memory:` | SQLite in-memory database |
| PostgreSQL URL | `postgres://user:pass@localhost/myapp` | Extracts `myapp` as filename `myapp.db` |
| PG key=value | `host=localhost dbname=myapp` | Extracts `myapp` as filename `myapp.db` |

## DDL Type Mappings

| PostgreSQL | SQLite |
|---|---|
| `SERIAL` / `BIGSERIAL` / `SMALLSERIAL` | `INTEGER PRIMARY KEY AUTOINCREMENT` |
| `BOOLEAN` / `BOOL` | `INTEGER` |
| `VARCHAR(n)` / `CHARACTER VARYING(n)` | `TEXT` |
| `CHAR(n)` / `CHARACTER(n)` | `TEXT` |
| `TIMESTAMP` / `TIMESTAMP WITH TIME ZONE` / `TIMESTAMPTZ` | `TEXT` |
| `DATE` | `TEXT` |
| `TIME` / `TIME WITH TIME ZONE` / `TIMETZ` | `TEXT` |
| `UUID` | `TEXT` |
| `BYTEA` | `BLOB` |
| `JSON` / `JSONB` | `TEXT` |
| `SMALLINT` / `INT2` | `INTEGER` |
| `INTEGER` / `INT` / `INT4` | `INTEGER` |
| `BIGINT` / `INT8` | `INTEGER` |
| `REAL` / `FLOAT4` | `REAL` |
| `DOUBLE PRECISION` / `FLOAT8` | `REAL` |
| `NUMERIC(p,s)` / `DECIMAL(p,s)` | `REAL` |
| `TEXT` | `TEXT` |
| `INTERVAL` | `TEXT` |

## Expression Translations

| PostgreSQL | SQLite |
|---|---|
| `expr::type` | `CAST(expr AS mapped_type)` |
| `ILIKE` | `LIKE` |
| `TRUE` | `1` |
| `FALSE` | `0` |
| `E'escape\nstring'` | `'escape' \|\| char(10) \|\| 'string'` |
| `expr IS TRUE` | `expr = 1` |
| `expr IS FALSE` | `expr = 0` |
| `expr IS NOT TRUE` | `expr != 1` |
| `expr IS NOT FALSE` | `expr != 0` |
| `$1`, `$2`, ... | `?` |
| `DEFAULT NOW()` | `DEFAULT (datetime('now'))` |

## Function Translations

| PostgreSQL | SQLite |
|---|---|
| `NOW()` | `datetime('now')` |
| `CURRENT_DATE` | `date('now')` |
| `CURRENT_TIME` | `time('now')` |
| `CURRENT_TIMESTAMP` | `datetime('now')` |
| `date_trunc('day', expr)` | `date(expr)` |
| `date_trunc('hour', expr)` | `strftime('%Y-%m-%d %H:00:00', expr)` |
| `date_trunc('minute', expr)` | `strftime('%Y-%m-%d %H:%M:00', expr)` |
| `date_trunc('month', expr)` | `strftime('%Y-%m-01', expr)` |
| `date_trunc('year', expr)` | `strftime('%Y-01-01', expr)` |
| `EXTRACT(field FROM expr)` | `CAST(strftime(fmt, expr) AS INTEGER)` |
| `date_part('field', expr)` | `CAST(strftime(fmt, expr) AS INTEGER)` |
| `left(str, n)` | `substr(str, 1, n)` |
| `right(str, n)` | `substr(str, -n)` |
| `concat(a, b, ...)` | `(COALESCE(a,'') \|\| COALESCE(b,'') \|\| ...)` |
| `string_agg(expr, sep)` | `group_concat(expr, sep)` |
| `array_agg(expr)` | `json_group_array(expr)` |
| `to_char(ts, fmt)` | `strftime(mapped_fmt, ts)` |

## Registered PG-Compatible Functions

These functions are registered as SQLite custom functions and can be called directly:

| Function | Description |
|---|---|
| `gen_random_uuid()` | Returns a random UUID v4 string |
| `md5(string)` | Returns the hex-encoded MD5 hash |
| `split_part(string, delimiter, field)` | Returns the nth field (1-indexed) |
| `pg_typeof(expr)` | Returns the SQLite type name of the expression |

## File Structure

```
go-postgres/
  go.mod                  Module definition (single dependency: modernc.org/sqlite)
  driver.go               Driver registration, conn/stmt/rows/tx/result wrappers, DSN parsing
  driver_go18.go          Go 1.8+ context-aware interfaces
  translate.go            Core tokenizer + translation pipeline
  translate_ddl.go        DDL type mappings (SERIAL, BOOLEAN, VARCHAR, etc.)
  translate_expr.go       Expression translations (::cast, ILIKE, TRUE/FALSE, E'strings')
  translate_func.go       Function translations (NOW, date_trunc, EXTRACT, etc.)
  pgfuncs.go              PG-compat functions registered in SQLite
  translate_test.go       Unit tests for all translations
  driver_test.go          Integration tests (full SQL round-trips)
  example/main.go         Usage example
  ROADMAP.md              Phase 2 and 3 plans
```

## License

MIT
