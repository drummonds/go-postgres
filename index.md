# go-postgres

A lightweight, pure Go `database/sql` driver that accepts PostgreSQL SQL syntax but executes against SQLite under the hood via [ncruces/go-sqlite3](https://github.com/ncruces/go-sqlite3).

This lets Go applications written for PostgreSQL run against a local SQLite file — ideal for testing, embedded use, CLI tools, and development environments. Files remain SQLite-compatible.

The driver registers as `"pglike"` to avoid conflicts with existing PG drivers (`lib/pq`, `pgx`).

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

## Installation

```bash
go get github.com/drummonds/go-postgres
```

## Links

- [Source (Codeberg)](https://codeberg.org/hum3/go-postgres)
- [Mirror (GitHub)](https://github.com/drummonds/go-postgres)
