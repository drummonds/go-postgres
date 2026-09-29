package pglike

import (
	"database/sql"
	"errors"
	"testing"
)

// schemaFixture: two schemas plus a public table sharing a name with one in
// crm, a cross-schema FK and a cross-schema view.
func schemaFixture(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	for _, s := range []string{
		`CREATE SCHEMA crm`,
		`CREATE SCHEMA IF NOT EXISTS sales`,
		`CREATE TABLE crm.customers (id INTEGER PRIMARY KEY, name VARCHAR(50) NOT NULL)`,
		`CREATE INDEX idx_customers_name ON crm.customers (name)`,
		`CREATE TABLE sales.orders (
			id INTEGER PRIMARY KEY,
			customer_id INTEGER NOT NULL REFERENCES crm.customers(id),
			total BIGINT NOT NULL
		)`,
		`CREATE TABLE customers (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE VIEW sales.order_names AS
			SELECT o.id, c.name FROM sales.orders o JOIN crm.customers c ON c.id = o.customer_id`,
		`INSERT INTO crm.customers (id, name) VALUES (1, 'Ada'), (2, 'Grace')`,
		`INSERT INTO customers (id, name) VALUES (1, 'public Ada')`,
		`INSERT INTO sales.orders (id, customer_id, total) VALUES (10, 1, 500), (11, 2, 700)`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup: %s\n%v", s, err)
		}
	}
	return db
}

func sqlState(err error) string {
	var pe *PGError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func TestSchemaCrossSchemaQueries(t *testing.T) {
	db := schemaFixture(t)

	var name string
	if err := db.QueryRow(`SELECT crm.customers.name FROM crm.customers WHERE crm.customers.id = 1`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Ada" {
		t.Errorf("crm.customers name = %q, want Ada", name)
	}
	if err := db.QueryRow(`SELECT name FROM customers WHERE id = 1`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "public Ada" {
		t.Errorf("public customers name = %q, want public Ada", name)
	}

	var total int64
	err := db.QueryRow(`SELECT sum(o.total) FROM sales.orders o
		JOIN crm.customers ON customers.id = o.customer_id
		WHERE customers.name = $1`, "Grace").Scan(&total)
	if err != nil {
		t.Fatal(err)
	}
	if total != 700 {
		t.Errorf("join total = %d, want 700", total)
	}

	if err := db.QueryRow(`SELECT name FROM sales.order_names WHERE id = 10`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Ada" {
		t.Errorf("view name = %q, want Ada", name)
	}

	if _, err := db.Exec(`UPDATE sales.orders SET total = total + 1 WHERE sales.orders.id = 10`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM sales.orders WHERE id = 11`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sales.orders`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("orders = %d, want 1", n)
	}
}

func TestSchemaCrossSchemaForeignKey(t *testing.T) {
	db := schemaFixture(t)
	_, err := db.Exec(`INSERT INTO sales.orders (id, customer_id, total) VALUES (12, 99, 1)`)
	if got := sqlState(err); got != "23503" {
		t.Errorf("FK violation SQLSTATE = %q (%v), want 23503", got, err)
	}
}

func TestSchemaTransactionSpansSchemas(t *testing.T) {
	db := schemaFixture(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO crm.customers (id, name) VALUES (3, 'Linus')`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO sales.orders (id, customer_id, total) VALUES (13, 3, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM crm.customers) + (SELECT count(*) FROM sales.orders)`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("rows after rollback = %d, want 4", n)
	}
}

func TestSchemaCatalogs(t *testing.T) {
	db := schemaFixture(t)

	rows, err := db.Query(`SELECT table_schema || '.' || table_name FROM information_schema.tables
		WHERE table_schema IN ('crm', 'sales', 'public') ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	got := collectStrings(t, rows)
	want := []string{"crm.customers", "public.customers", "sales.order_names", "sales.orders"}
	if !equalSlices(got, want) {
		t.Errorf("tables = %v, want %v", got, want)
	}

	rows, err = db.Query(`SELECT schema_name FROM information_schema.schemata ORDER BY schema_name`)
	if err != nil {
		t.Fatal(err)
	}
	got = collectStrings(t, rows)
	want = []string{"crm", "public", "sales"}
	if !equalSlices(got, want) {
		t.Errorf("schemata = %v, want %v", got, want)
	}

	var dataType string
	if err := db.QueryRow(`SELECT data_type FROM information_schema.columns
		WHERE table_schema = 'crm' AND table_name = 'customers' AND column_name = 'name'`).Scan(&dataType); err != nil {
		t.Fatal(err)
	}
	if dataType != "character varying" {
		t.Errorf("crm.customers.name data_type = %q", dataType)
	}

	var tbl string
	if err := db.QueryRow(`SELECT tablename FROM pg_tables WHERE schemaname = 'sales'`).Scan(&tbl); err != nil {
		t.Fatal(err)
	}
	if tbl != "orders" {
		t.Errorf("pg_tables sales = %q, want orders", tbl)
	}

	rows, err = db.Query(`SELECT indexname || ' ' || indexdef FROM pg_indexes
		WHERE schemaname = 'crm' AND tablename = 'customers' AND indexname = 'customers_pkey'`)
	if err != nil {
		t.Fatal(err)
	}
	got = collectStrings(t, rows)
	want = []string{"customers_pkey CREATE UNIQUE INDEX customers_pkey ON crm.customers USING btree (id)"}
	if !equalSlices(got, want) {
		t.Errorf("pkey = %v, want %v", got, want)
	}
	if err := db.QueryRow(`SELECT indexname FROM pg_indexes
		WHERE schemaname = 'crm' AND indexname LIKE 'idx%'`).Scan(&tbl); err != nil {
		t.Fatal(err)
	}
	if tbl != "idx_customers_name" {
		t.Errorf("crm index = %q, want idx_customers_name", tbl)
	}

	// The cross-schema FK names its parent in crm.
	var fkSchema, fkTable, uniqSchema, uniqName string
	err = db.QueryRow(`SELECT ccu.table_schema, ccu.table_name, rc.unique_constraint_schema, rc.unique_constraint_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.constraint_column_usage ccu ON ccu.constraint_name = tc.constraint_name
		JOIN information_schema.referential_constraints rc ON rc.constraint_name = tc.constraint_name
		WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_schema = 'sales' AND tc.table_name = 'orders'`).
		Scan(&fkSchema, &fkTable, &uniqSchema, &uniqName)
	if err != nil {
		t.Fatal(err)
	}
	if fkSchema != "crm" || fkTable != "customers" || uniqSchema != "crm" || uniqName != "customers_pkey" {
		t.Errorf("fk parent = %s.%s (%s.%s)", fkSchema, fkTable, uniqSchema, uniqName)
	}
}

func TestSchemaDDL(t *testing.T) {
	db := schemaFixture(t)

	cases := []struct {
		sql, state string
	}{
		{`CREATE SCHEMA crm`, "42P06"},
		{`CREATE SCHEMA public`, "42P06"},
		{`CREATE SCHEMA IF NOT EXISTS crm`, ""},
		{`DROP SCHEMA nosuch`, "3F000"},
		{`DROP SCHEMA IF EXISTS nosuch`, ""},
		{`DROP SCHEMA crm`, "2BP01"},
		{`DROP SCHEMA crm RESTRICT`, "2BP01"},
		{`DROP SCHEMA public`, "0A000"},
	}
	for _, c := range cases {
		_, err := db.Exec(c.sql)
		if got := sqlState(err); got != c.state {
			t.Errorf("%s: SQLSTATE %q (%v), want %q", c.sql, got, err, c.state)
		}
	}

	// An empty schema drops without CASCADE.
	if _, err := db.Exec(`CREATE SCHEMA empty; DROP SCHEMA empty`); err != nil {
		t.Fatal(err)
	}

	// CASCADE drops the schema's tables, views and indexes, and nothing else.
	if _, err := db.Exec(`DROP SCHEMA sales CASCADE`); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT schema_name FROM information_schema.schemata ORDER BY schema_name`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := collectStrings(t, rows), []string{"crm", "public"}; !equalSlices(got, want) {
		t.Errorf("schemata after drop = %v, want %v", got, want)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'sales'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("sales still has %d relations", n)
	}
	if err := db.QueryRow(`SELECT count(*) FROM crm.customers`).Scan(&n); err != nil || n != 2 {
		t.Errorf("crm.customers after dropping sales: n=%d err=%v", n, err)
	}
	if _, err := db.Exec(`CREATE SCHEMA sales`); err != nil {
		t.Errorf("recreate sales: %v", err)
	}
}
