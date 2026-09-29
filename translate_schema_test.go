package pglike

import "testing"

// Decision table for schema-qualified name rewriting (#20). A relation in a
// non-public schema is stored as the single SQLite table "schema.name";
// public.name is plain name. Two-part names are rewritten only where a
// relation is expected, so alias.col is never touched; three-part
// schema.rel.col always refers to a relation's column.
func TestTranslateSchemas(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		// SELECT sources get an implicit alias so bare rel.col keeps resolving.
		{"from", `SELECT * FROM crm.customers`, `SELECT * FROM "crm.customers" AS customers`},
		{"from alias", `SELECT c.id FROM crm.customers c`, `SELECT c.id FROM "crm.customers" c`},
		{"from AS alias", `SELECT c.id FROM crm.customers AS c`, `SELECT c.id FROM "crm.customers" AS c`},
		{"three-part column", `SELECT crm.customers.id FROM crm.customers`, `SELECT customers.id FROM "crm.customers" AS customers`},
		{"from list", `SELECT * FROM a.x, b.y WHERE x.id = y.id`, `SELECT * FROM "a.x" AS x, "b.y" AS y WHERE x.id = y.id`},
		{"join", `SELECT * FROM crm.a JOIN sales.b ON a.id = b.a_id`, `SELECT * FROM "crm.a" AS a JOIN "sales.b" AS b ON a.id = b.a_id`},
		{"subquery", `SELECT * FROM (SELECT id FROM crm.t) s`, `SELECT * FROM (SELECT id FROM "crm.t" AS t) s`},
		{"public stripped", `SELECT * FROM public.t JOIN public.u ON t.id = u.id`, `SELECT * FROM t JOIN u ON t.id = u.id`},
		{"public three-part", `SELECT public.t.id FROM public.t`, `SELECT t.id FROM t`},
		{"case folded", `SELECT * FROM CRM.Customers`, `SELECT * FROM "crm.customers" AS customers`},
		{"quoted parts", `SELECT * FROM "My Schema"."T"`, `SELECT * FROM "My Schema.T" AS "T"`},

		// Not relations: left alone.
		{"alias.col", `SELECT t.id FROM t WHERE t.id = 1`, `SELECT t.id FROM t WHERE t.id = 1`},
		{"extract", `SELECT EXTRACT(YEAR FROM o.created) FROM o`, `SELECT EXTRACT(YEAR FROM o.created) FROM o`},
		{"distinct from", `SELECT 1 FROM t WHERE a IS DISTINCT FROM t.b`, `SELECT 1 FROM t WHERE a IS DISTINCT FROM t.b`},
		{"string", `SELECT 'crm.t'`, `SELECT 'crm.t'`},
		{"upsert DO UPDATE", `INSERT INTO t (id) VALUES (1) ON CONFLICT (id) DO UPDATE SET v = excluded.v`, `INSERT INTO t (id) VALUES (1) ON CONFLICT (id) DO UPDATE SET v = excluded.v`},

		// DML targets.
		{"insert", `INSERT INTO crm.t (id) VALUES (1)`, `INSERT INTO "crm.t" AS t (id) VALUES (1)`},
		{"update", `UPDATE crm.t SET x = 1 WHERE crm.t.id = 2`, `UPDATE "crm.t" AS t SET x = 1 WHERE t.id = 2`},
		{"delete", `DELETE FROM crm.t WHERE id = 1`, `DELETE FROM "crm.t" AS t WHERE id = 1`},

		// DDL: no implicit alias.
		{"create table", `CREATE TABLE crm.t (id INT)`, `CREATE TABLE "crm.t" (id INT)`},
		{"create table if not exists", `CREATE TABLE IF NOT EXISTS crm.t (id INT)`, `CREATE TABLE IF NOT EXISTS "crm.t" (id INT)`},
		{"references", `CREATE TABLE crm.o (cid INT REFERENCES crm.c(id))`, `CREATE TABLE "crm.o" (cid INT REFERENCES "crm.c"(id))`},
		{"alter table", `ALTER TABLE crm.t ADD COLUMN x INT`, `ALTER TABLE "crm.t" ADD COLUMN x INT`},
		{"drop table", `DROP TABLE IF EXISTS crm.t`, `DROP TABLE IF EXISTS "crm.t"`},
		{"create view", `CREATE VIEW crm.v AS SELECT * FROM crm.t`, `CREATE VIEW "crm.v" AS SELECT * FROM "crm.t" AS t`},
		{"drop view", `DROP VIEW crm.v`, `DROP VIEW "crm.v"`},
		// PG puts an index in its table's schema; SQLite index names are global.
		{"create index", `CREATE INDEX idx ON crm.t (x)`, `CREATE INDEX "crm.idx" ON "crm.t" (x)`},
		{"create unique index", `CREATE UNIQUE INDEX IF NOT EXISTS idx ON crm.t (x)`, `CREATE UNIQUE INDEX IF NOT EXISTS "crm.idx" ON "crm.t" (x)`},
		{"create index public", `CREATE INDEX idx ON public.t (x)`, `CREATE INDEX idx ON t (x)`},
		{"drop index", `DROP INDEX crm.idx`, `DROP INDEX "crm.idx"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Reassemble(translateSchemas(Tokenize(tt.in)))
			if got != tt.want {
				t.Errorf("\n  in:   %s\n  got:  %s\n  want: %s", tt.in, got, tt.want)
			}
		})
	}
}
