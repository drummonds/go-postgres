package pglike

import (
	"database/sql"
	"sort"
	"testing"
)

// catalogFixture creates a small schema covering the cases lofidb needs:
// PK, NOT NULL, FK with action rules, UNIQUE, multi-column index,
// composite PK, multi-column FK.
func catalogFixture(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	stmts := []string{
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			display_name TEXT
		)`,
		`CREATE TABLE posts (
			id INTEGER PRIMARY KEY,
			author_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE ON UPDATE NO ACTION,
			title TEXT NOT NULL,
			body TEXT
		)`,
		`CREATE INDEX idx_posts_author ON posts(author_id)`,
		`CREATE INDEX idx_posts_title_body ON posts(title, body)`,
		`CREATE TABLE tags (
			post_id INTEGER NOT NULL,
			tag TEXT NOT NULL,
			PRIMARY KEY (post_id, tag),
			FOREIGN KEY (post_id) REFERENCES posts(id)
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup: %s\n%v", s, err)
		}
	}
	return db
}

func collectStrings(t *testing.T, rows *sql.Rows) []string {
	t.Helper()
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCatalogTables(t *testing.T) {
	db := catalogFixture(t)
	rows, err := db.Query(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'
		ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	got := collectStrings(t, rows)
	want := []string{"posts", "tags", "users"}
	if !equalSlices(got, want) {
		t.Errorf("tables = %v, want %v", got, want)
	}
}

func TestCatalogColumns(t *testing.T) {
	db := catalogFixture(t)

	rows, err := db.Query(`SELECT column_name, data_type, is_nullable, ordinal_position
		FROM information_schema.columns
		WHERE table_name = 'posts' ORDER BY ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	type col struct {
		name, dtype, nullable string
		pos                   int
	}
	var got []col
	for rows.Next() {
		var c col
		if err := rows.Scan(&c.name, &c.dtype, &c.nullable, &c.pos); err != nil {
			t.Fatal(err)
		}
		got = append(got, c)
	}
	want := []col{
		{"id", "integer", "NO", 1},
		{"author_id", "integer", "NO", 2},
		{"title", "text", "NO", 3},
		{"body", "text", "YES", 4},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d cols, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("col[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

// data_type and udt_name report the PG type as declared, not the SQLite type
// the DDL translator rewrote it to (#19).
func TestCatalogColumnPGTypes(t *testing.T) {
	db := openTestDB(t)
	for _, s := range []string{
		`CREATE TABLE typed (
			a BIGINT, b INTEGER, c INT, d SMALLINT, e TEXT,
			f VARCHAR(100), g CHARACTER VARYING(20), h CHAR(3), i BOOLEAN,
			j TIMESTAMPTZ, k TIMESTAMP WITH TIME ZONE, l TIMESTAMP, m DATE,
			n UUID, o NUMERIC(10,2), p DOUBLE PRECISION, q REAL, r BYTEA,
			s JSONB, t INTERVAL, "Mixed Case" VARCHAR(5)
		)`,
		`ALTER TABLE typed ADD COLUMN added TIMESTAMPTZ DEFAULT now()`,
		`CREATE TABLE serials (id BIGSERIAL PRIMARY KEY)`,
		`CREATE TABLE serials4 (id SERIAL PRIMARY KEY)`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup: %s\n%v", s, err)
		}
	}

	want := map[string][2]string{
		"typed.a":          {"bigint", "int8"},
		"typed.b":          {"integer", "int4"},
		"typed.c":          {"integer", "int4"},
		"typed.d":          {"smallint", "int2"},
		"typed.e":          {"text", "text"},
		"typed.f":          {"character varying", "varchar"},
		"typed.g":          {"character varying", "varchar"},
		"typed.h":          {"character", "bpchar"},
		"typed.i":          {"boolean", "bool"},
		"typed.j":          {"timestamp with time zone", "timestamptz"},
		"typed.k":          {"timestamp with time zone", "timestamptz"},
		"typed.l":          {"timestamp without time zone", "timestamp"},
		"typed.m":          {"date", "date"},
		"typed.n":          {"uuid", "uuid"},
		"typed.o":          {"numeric", "numeric"},
		"typed.p":          {"double precision", "float8"},
		"typed.q":          {"real", "float4"},
		"typed.r":          {"bytea", "bytea"},
		"typed.s":          {"jsonb", "jsonb"},
		"typed.t":          {"interval", "interval"},
		"typed.Mixed Case": {"character varying", "varchar"},
		"typed.added":      {"timestamp with time zone", "timestamptz"},
		"serials.id":       {"bigint", "int8"},
		"serials4.id":      {"integer", "int4"},
	}
	rows, err := db.Query(`SELECT table_name, column_name, data_type, udt_name
		FROM information_schema.columns
		WHERE table_name IN ('typed', 'serials', 'serials4')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var tbl, col, dt, udt string
		if err := rows.Scan(&tbl, &col, &dt, &udt); err != nil {
			t.Fatal(err)
		}
		key := tbl + "." + col
		w, ok := want[key]
		if !ok {
			t.Errorf("unexpected column %s", key)
			continue
		}
		seen++
		if dt != w[0] || udt != w[1] {
			t.Errorf("%s = (%q, %q), want (%q, %q)", key, dt, udt, w[0], w[1])
		}
	}
	if seen != len(want) {
		t.Errorf("saw %d columns, want %d", seen, len(want))
	}
}

func TestCatalogPrimaryKey(t *testing.T) {
	db := catalogFixture(t)

	// users has single-col PK (id).
	rows, err := db.Query(`SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_name = 'users'
		ORDER BY kcu.ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	got := collectStrings(t, rows)
	if !equalSlices(got, []string{"id"}) {
		t.Errorf("users PK cols = %v, want [id]", got)
	}

	// tags has composite PK (post_id, tag).
	rows, err = db.Query(`SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_name = 'tags'
		ORDER BY kcu.ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	got = collectStrings(t, rows)
	if !equalSlices(got, []string{"post_id", "tag"}) {
		t.Errorf("tags PK cols = %v, want [post_id tag]", got)
	}
}

func TestCatalogForeignKeys(t *testing.T) {
	db := catalogFixture(t)
	rows, err := db.Query(`SELECT
			kcu.column_name, ccu.table_name, ccu.column_name,
			rc.update_rule, rc.delete_rule
		FROM information_schema.referential_constraints rc
		JOIN information_schema.key_column_usage kcu
		  ON rc.constraint_name = kcu.constraint_name
		JOIN information_schema.constraint_column_usage ccu
		  ON rc.unique_constraint_name = ccu.constraint_name
		WHERE kcu.table_name = 'posts'
		ORDER BY kcu.column_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type fk struct {
		col, refTable, refCol, onUpd, onDel string
	}
	var got []fk
	for rows.Next() {
		var f fk
		if err := rows.Scan(&f.col, &f.refTable, &f.refCol, &f.onUpd, &f.onDel); err != nil {
			t.Fatal(err)
		}
		got = append(got, f)
	}
	want := []fk{{"author_id", "users", "id", "NO ACTION", "CASCADE"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("posts FKs = %+v, want %+v", got, want)
	}
}

func TestCatalogIndexes(t *testing.T) {
	db := catalogFixture(t)
	rows, err := db.Query(`SELECT indexname FROM pg_indexes
		WHERE tablename = 'posts' ORDER BY indexname`)
	if err != nil {
		t.Fatal(err)
	}
	got := collectStrings(t, rows)
	want := []string{"idx_posts_author", "idx_posts_title_body", "posts_pkey"}
	if !equalSlices(got, want) {
		t.Errorf("posts indexes = %v, want %v", got, want)
	}

	// Index columns via the pglike-specific helper view.
	rows, err = db.Query(`SELECT column_name FROM pg_index_columns
		WHERE indexname = 'idx_posts_title_body' ORDER BY ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	got = collectStrings(t, rows)
	want = []string{"title", "body"}
	if !equalSlices(got, want) {
		t.Errorf("idx_posts_title_body cols = %v, want %v", got, want)
	}
}

// PK indexes are listed as <table>_pkey with PG's indexdef shape, whether
// SQLite backs the PK with an autoindex (composite) or the rowid (INTEGER
// PRIMARY KEY, no autoindex at all).
func TestCatalogIndexesPrimaryKey(t *testing.T) {
	db := catalogFixture(t)
	cases := []struct{ table, def string }{
		{"posts", "CREATE UNIQUE INDEX posts_pkey ON public.posts USING btree (id)"},
		{"tags", "CREATE UNIQUE INDEX tags_pkey ON public.tags USING btree (post_id, tag)"},
	}
	for _, c := range cases {
		var def string
		err := db.QueryRow(`SELECT indexdef FROM pg_indexes
			WHERE schemaname = 'public' AND tablename = $1 AND indexname = $2`,
			c.table, c.table+"_pkey").Scan(&def)
		if err != nil {
			t.Fatalf("%s_pkey: %v", c.table, err)
		}
		if def != c.def {
			t.Errorf("%s_pkey indexdef = %q, want %q", c.table, def, c.def)
		}
	}

	// A table without a PK gets no _pkey row.
	if _, err := db.Exec(`CREATE TABLE nopk (a TEXT)`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM pg_indexes WHERE tablename = 'nopk'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("nopk has %d index rows, want 0", n)
	}
}

func TestCatalogHidesInternals(t *testing.T) {
	db := catalogFixture(t)
	// _sequences and any pglike-internal tables must not show up.
	rows, err := db.Query(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = current_schema()`)
	if err != nil {
		t.Fatal(err)
	}
	got := collectStrings(t, rows)
	for _, n := range got {
		if n == "_sequences" || (len(n) > 7 && n[:8] == "_pglike_") {
			t.Errorf("internal table %q leaked into information_schema.tables", n)
		}
	}
}

func TestCatalogRewriterIgnoresStrings(t *testing.T) {
	db := catalogFixture(t)
	// A string literal containing a catalog reference must not be rewritten.
	var s string
	if err := db.QueryRow(`SELECT 'information_schema.columns is a view'`).Scan(&s); err != nil {
		t.Fatal(err)
	}
	if s != "information_schema.columns is a view" {
		t.Errorf("got %q", s)
	}
}

func TestCatalogTranslateIdempotent(t *testing.T) {
	// Translating already-mangled SQL must be a no-op.
	src := `SELECT * FROM _pglike_information_schema_columns`
	out, err := Translate(src)
	if err != nil {
		t.Fatal(err)
	}
	// The translator may add a trailing semicolon-handling or whitespace
	// normalisation; we just check the mangled name is preserved.
	if !contains(out, "_pglike_information_schema_columns") {
		t.Errorf("translation lost mangled name: %q", out)
	}
}

func TestCatalogQualifiedSurvivesIfPrefixed(t *testing.T) {
	// `mydb.pg_indexes` must not be rewritten — pg_indexes is the RHS of
	// a dotted reference and we shouldn't touch it.
	src := `SELECT * FROM mydb.pg_indexes`
	out, err := Translate(src)
	if err != nil {
		t.Fatal(err)
	}
	if contains(out, "_pglike_pg_indexes") {
		t.Errorf("rewrote dotted RHS: %q", out)
	}
}

// equalSlices compares two []string for equality (order-sensitive).
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(s, substr string) bool {
	return indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// Smoke check that ordering is stable in our equalSlices helper.
func TestEqualSlices(t *testing.T) {
	a := []string{"a", "b", "c"}
	b := []string{"a", "b", "c"}
	sort.Strings(a)
	sort.Strings(b)
	if !equalSlices(a, b) {
		t.Fail()
	}
}

// TestCatalogExplorerQueries runs the exact catalog queries the gobank DB
// explorer issues against real PostgreSQL (issue #17), scanning into the
// same Go types, so the explorer needs no pglike-specific branches.
func TestCatalogExplorerQueries(t *testing.T) {
	db := openTestDB(t)
	for _, s := range []string{
		`CREATE TABLE users (id SERIAL PRIMARY KEY, name VARCHAR(100) NOT NULL, email TEXT UNIQUE)`,
		`CREATE TABLE orders (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, total NUMERIC(10,2))`,
		`CREATE INDEX idx_orders_user ON orders (user_id)`,
		`CREATE VIEW v_users AS SELECT id, name FROM users`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup: %s\n%v", s, err)
		}
	}

	// 1. Table list.
	var tables []string
	rows, err := db.Query(`SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename`)
	if err != nil {
		t.Fatalf("pg_tables: %v", err)
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, n)
	}
	rows.Close()
	if !equalSlices(tables, []string{"orders", "users"}) {
		t.Errorf("pg_tables = %v, want [orders users]", tables)
	}

	// 2. Columns with PK flag; $1 is reused.
	type col struct {
		pos            int
		name, typ, def string
		notNull, isPK  bool
	}
	var cols []col
	rows, err = db.Query(`SELECT c.ordinal_position, c.column_name, c.data_type,
       c.is_nullable = 'NO', COALESCE(c.column_default, ''), COALESCE(pk.is_pk, false)
FROM information_schema.columns c
LEFT JOIN (SELECT kcu.column_name, true AS is_pk
    FROM information_schema.table_constraints tc
    JOIN information_schema.key_column_usage kcu
      ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
    WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = 'public' AND tc.table_name = $1
) pk ON pk.column_name = c.column_name
WHERE c.table_schema = 'public' AND c.table_name = $1
ORDER BY c.ordinal_position`, "orders")
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	for rows.Next() {
		var c col
		if err := rows.Scan(&c.pos, &c.name, &c.typ, &c.notNull, &c.def, &c.isPK); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, c)
	}
	rows.Close()
	if len(cols) != 3 {
		t.Fatalf("columns = %+v, want 3 rows", cols)
	}
	if cols[0].name != "id" || !cols[0].isPK || !cols[0].notNull || cols[0].def != "gen_random_uuid()" {
		t.Errorf("id column = %+v", cols[0])
	}
	if cols[1].name != "user_id" || cols[1].isPK || !cols[1].notNull {
		t.Errorf("user_id column = %+v", cols[1])
	}
	if cols[2].name != "total" || cols[2].isPK || cols[2].notNull {
		t.Errorf("total column = %+v", cols[2])
	}

	// 3. Foreign keys with rules.
	var fkName, fkCol, fkTable, fkRef, upd, del string
	err = db.QueryRow(`SELECT tc.constraint_name, kcu.column_name, ccu.table_name, ccu.column_name,
       rc.update_rule, rc.delete_rule
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
JOIN information_schema.constraint_column_usage ccu
  ON ccu.constraint_name = tc.constraint_name AND ccu.table_schema = tc.table_schema
JOIN information_schema.referential_constraints rc
  ON rc.constraint_name = tc.constraint_name AND rc.constraint_schema = tc.table_schema
WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_schema = 'public' AND tc.table_name = $1
ORDER BY tc.constraint_name, kcu.ordinal_position`, "orders").
		Scan(&fkName, &fkCol, &fkTable, &fkRef, &upd, &del)
	if err != nil {
		t.Fatalf("foreign keys: %v", err)
	}
	if fkCol != "user_id" || fkTable != "users" || fkRef != "id" || del != "CASCADE" || upd != "NO ACTION" {
		t.Errorf("fk = %s %s -> %s.%s update=%s delete=%s", fkName, fkCol, fkTable, fkRef, upd, del)
	}

	// 4. Indexes.
	var idxName, idxDef string
	err = db.QueryRow(`SELECT indexname, indexdef FROM pg_indexes WHERE schemaname = 'public' AND tablename = $1 ORDER BY indexname`, "orders").
		Scan(&idxName, &idxDef)
	if err != nil {
		t.Fatalf("pg_indexes: %v", err)
	}
	if idxName != "idx_orders_user" || idxDef == "" {
		t.Errorf("index = %s %q", idxName, idxDef)
	}

	// Views are listed by pg_views, not pg_tables.
	var viewName string
	if err := db.QueryRow(`SELECT viewname FROM pg_catalog.pg_views WHERE schemaname = 'public'`).Scan(&viewName); err != nil {
		t.Fatalf("pg_views: %v", err)
	}
	if viewName != "v_users" {
		t.Errorf("pg_views = %q, want v_users", viewName)
	}
}
