package pglike

import (
	"testing"
)

// TestForeignKeyEnforced verifies that inserting a row with a non-existent
// foreign key value is rejected. This matches PostgreSQL behaviour.
// Note: ncruces/go-sqlite3 compiles with SQLITE_DEFAULT_FOREIGN_KEYS=1,
// so foreign keys are enforced by default.
func TestForeignKeyEnforced(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`CREATE TABLE parents (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL
	)`)
	if err != nil {
		t.Fatalf("CREATE TABLE parents: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE children (
		id SERIAL PRIMARY KEY,
		parent_id INTEGER NOT NULL REFERENCES parents(id),
		name VARCHAR(100) NOT NULL
	)`)
	if err != nil {
		t.Fatalf("CREATE TABLE children: %v", err)
	}

	// Insert into child with a parent_id that does not exist.
	// PostgreSQL would reject this; SQLite without foreign_keys pragma allows it.
	_, err = db.Exec("INSERT INTO children (parent_id, name) VALUES (999, 'orphan')")
	if err == nil {
		t.Error("INSERT with non-existent parent_id succeeded; expected foreign key violation")
	}
}

// TestForeignKeyDeleteRestrict verifies that deleting a parent row that is
// referenced by a child row is rejected (default RESTRICT behaviour).
func TestForeignKeyDeleteRestrict(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`CREATE TABLE authors (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL
	)`)
	if err != nil {
		t.Fatalf("CREATE TABLE authors: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE books (
		id SERIAL PRIMARY KEY,
		author_id INTEGER NOT NULL REFERENCES authors(id),
		title VARCHAR(200) NOT NULL
	)`)
	if err != nil {
		t.Fatalf("CREATE TABLE books: %v", err)
	}

	_, err = db.Exec("INSERT INTO authors (id, name) VALUES (1, 'Tolkien')")
	if err != nil {
		t.Fatalf("INSERT author: %v", err)
	}

	_, err = db.Exec("INSERT INTO books (author_id, title) VALUES (1, 'The Hobbit')")
	if err != nil {
		t.Fatalf("INSERT book: %v", err)
	}

	// Deleting the referenced parent should fail.
	_, err = db.Exec("DELETE FROM authors WHERE id = 1")
	if err == nil {
		t.Error("DELETE of referenced parent succeeded; expected foreign key violation")
	}
}

// TestForeignKeyUpdateRestrict verifies that updating a parent's PK to a
// different value is rejected when a child still references the old value.
func TestForeignKeyUpdateRestrict(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`CREATE TABLE departments (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL
	)`)
	if err != nil {
		t.Fatalf("CREATE TABLE departments: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE employees (
		id SERIAL PRIMARY KEY,
		dept_id INTEGER NOT NULL REFERENCES departments(id),
		name VARCHAR(100) NOT NULL
	)`)
	if err != nil {
		t.Fatalf("CREATE TABLE employees: %v", err)
	}

	_, err = db.Exec("INSERT INTO departments (id, name) VALUES (1, 'Engineering')")
	if err != nil {
		t.Fatalf("INSERT department: %v", err)
	}

	_, err = db.Exec("INSERT INTO employees (dept_id, name) VALUES (1, 'Alice')")
	if err != nil {
		t.Fatalf("INSERT employee: %v", err)
	}

	// Changing the parent PK should fail while a child references it.
	_, err = db.Exec("UPDATE departments SET id = 99 WHERE id = 1")
	if err == nil {
		t.Error("UPDATE of referenced parent PK succeeded; expected foreign key violation")
	}
}

// TestForeignKeyValidInsert verifies that a valid foreign key insert succeeds
// (sanity check — should pass regardless of pragma).
func TestForeignKeyValidInsert(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`CREATE TABLE categories (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL
	)`)
	if err != nil {
		t.Fatalf("CREATE TABLE categories: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE products (
		id SERIAL PRIMARY KEY,
		category_id INTEGER NOT NULL REFERENCES categories(id),
		name VARCHAR(100) NOT NULL
	)`)
	if err != nil {
		t.Fatalf("CREATE TABLE products: %v", err)
	}

	_, err = db.Exec("INSERT INTO categories (id, name) VALUES (1, 'Electronics')")
	if err != nil {
		t.Fatalf("INSERT category: %v", err)
	}

	_, err = db.Exec("INSERT INTO products (category_id, name) VALUES (1, 'Laptop')")
	if err != nil {
		t.Fatalf("INSERT product with valid FK: %v", err)
	}
}
