package pglike

import (
	"database/sql"
	"testing"
)

func TestTranslateNumericAggregateOverCast(t *testing.T) {
	cases := map[string]string{
		"SELECT sum(x::numeric) FROM t":           "SELECT pg_numeric_sum(pg_numeric(x)) FROM t",
		"SELECT AVG(CAST(x AS NUMERIC)) FROM t":   "SELECT pg_numeric_avg(pg_numeric(x)) FROM t",
		"SELECT min(x::numeric), max(x::numeric)": "SELECT pg_numeric_min(pg_numeric(x)), pg_numeric_max(pg_numeric(x))",
		"SELECT sum(x::numeric) / 2 FROM t":       "SELECT pg_numeric_div(pg_numeric_sum(pg_numeric(x)), 2) FROM t",
		"SELECT sum(x) FROM t":                    "SELECT sum(x) FROM t",
		"SELECT count(x::numeric) FROM t":         "SELECT count(pg_numeric(x)) FROM t",
	}
	for in, want := range cases {
		got, err := Translate(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Errorf("%s\n  got  %s\n  want %s", in, got, want)
		}
	}
}

// rewriteNumericAggregates rewrites sum/avg/min/max over a bare column
// reference when that column is NUMERIC in a table the statement names.
func TestRewriteNumericAggregates(t *testing.T) {
	numeric := map[string]map[string]bool{
		"ledger":     {"amount": true},
		"crm.ledger": {"amount": true},
		"counts":     {},
	}
	cases := map[string]string{
		"SELECT sum(amount) FROM ledger":                                  "SELECT pg_numeric_sum(amount) FROM ledger",
		"SELECT SUM(l.amount) FROM ledger AS l JOIN accounts a ON 1":      "SELECT pg_numeric_sum(l.amount) FROM ledger AS l JOIN accounts a ON 1",
		"SELECT min(amount), max(amount), avg(amount) FROM ledger":        "SELECT pg_numeric_min(amount), pg_numeric_max(amount), pg_numeric_avg(amount) FROM ledger",
		`SELECT sum(amount) FROM "crm.ledger"`:                            `SELECT pg_numeric_sum(amount) FROM "crm.ledger"`,
		"SELECT sum(n) FROM counts":                                       "SELECT sum(n) FROM counts",
		"SELECT sum(amount) FROM counts":                                  "SELECT sum(amount) FROM counts",
		"SELECT sum(amount * 2) FROM ledger":                              "SELECT sum(amount * 2) FROM ledger",
		"SELECT count(amount) FROM ledger":                                "SELECT count(amount) FROM ledger",
		"CREATE VIEW b AS SELECT sum(amount) AS s FROM ledger GROUP BY 1": "CREATE VIEW b AS SELECT pg_numeric_sum(amount) AS s FROM ledger GROUP BY 1",
		"INSERT INTO ledger VALUES (1)":                                   "INSERT INTO ledger VALUES (1)",
	}
	for in, want := range cases {
		got := rewriteNumericAggregates(in, numeric)
		if got != want {
			t.Errorf("%s\n  got  %s\n  want %s", in, got, want)
		}
	}
}

func TestNumericColumnsFromSchema(t *testing.T) {
	db := openTestDB(t)
	mustExec(t, db, `CREATE TABLE ledger (account VARCHAR(10), amount NUMERIC(20,7), fee DECIMAL)`)
	mustExec(t, db, `CREATE TABLE counts (n BIGINT)`)
	mustExec(t, db, `CREATE SCHEMA crm`)
	mustExec(t, db, `CREATE TABLE crm.ledger (amount NUMERIC)`)
	var cols string
	err := db.QueryRow(`SELECT group_concat(table_name || '.' || column_name, ',') FROM (
		SELECT table_name, column_name FROM information_schema.columns
		WHERE udt_name = 'numeric' ORDER BY table_name, column_name)`).Scan(&cols)
	if err != nil {
		t.Fatal(err)
	}
	if cols != "ledger.amount,ledger.amount,ledger.fee" {
		t.Logf("catalog view lists: %s", cols)
	}
	c := rawConn(t, db)
	got, err := c.numericColumns()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string]bool{"ledger": {"amount": true, "fee": true}, "crm.ledger": {"amount": true}}
	for table, cols := range want {
		for col := range cols {
			if !got[table][col] {
				t.Errorf("%s.%s not reported numeric: %v", table, col, got)
			}
		}
	}
	if got["counts"]["n"] {
		t.Errorf("counts.n reported numeric")
	}
	before := c.numericSchemaVersion
	mustExec(t, db, `ALTER TABLE counts ADD COLUMN total NUMERIC`)
	got, err = c.numericColumns()
	if err != nil {
		t.Fatal(err)
	}
	if !got["counts"]["total"] || c.numericSchemaVersion == before {
		t.Errorf("DDL must refresh the cache: %v (version %d -> %d)", got, before, c.numericSchemaVersion)
	}
}

func mustExec(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// rawConn pins one pool connection for the test and returns the driver conn
// behind it.
func rawConn(t *testing.T, db *sql.DB) *conn {
	t.Helper()
	sc, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sc.Close() })
	var c *conn
	if err := sc.Raw(func(dc any) error { c = dc.(*conn); return nil }); err != nil {
		t.Fatal(err)
	}
	return c
}
