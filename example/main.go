package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/drummonds/go-postgres"
)

func main() {
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create a table using PostgreSQL syntax
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		email VARCHAR(255) UNIQUE,
		active BOOLEAN DEFAULT TRUE,
		created_at TIMESTAMP DEFAULT (datetime('now'))
	)`)
	if err != nil {
		log.Fatal("CREATE TABLE:", err)
	}

	// Insert using positional parameters (translated from $1,$2 to ?)
	_, err = db.Exec("INSERT INTO users (name, email) VALUES (?, ?)", "Alice", "alice@example.com")
	if err != nil {
		log.Fatal("INSERT:", err)
	}
	_, err = db.Exec("INSERT INTO users (name, email) VALUES (?, ?)", "Bob", "bob@example.com")
	if err != nil {
		log.Fatal("INSERT:", err)
	}

	// Query
	rows, err := db.Query("SELECT id, name, email, active, created_at FROM users WHERE active = 1")
	if err != nil {
		log.Fatal("SELECT:", err)
	}
	defer rows.Close()

	fmt.Println("Users:")
	for rows.Next() {
		var id int64
		var name, email, createdAt string
		var active int64
		if err := rows.Scan(&id, &name, &email, &active, &createdAt); err != nil {
			log.Fatal("Scan:", err)
		}
		fmt.Printf("  id=%d name=%s email=%s active=%d created_at=%s\n", id, name, email, active, createdAt)
	}
	if err := rows.Err(); err != nil {
		log.Fatal("rows:", err)
	}

	// Demonstrate PG functions
	var uuid string
	db.QueryRow("SELECT gen_random_uuid()").Scan(&uuid)
	fmt.Printf("\nGenerated UUID: %s\n", uuid)

	var hash string
	db.QueryRow("SELECT md5('hello world')").Scan(&hash)
	fmt.Printf("MD5 of 'hello world': %s\n", hash)

	var part string
	db.QueryRow("SELECT split_part('one,two,three', ',', 2)").Scan(&part)
	fmt.Printf("split_part result: %s\n", part)
}
