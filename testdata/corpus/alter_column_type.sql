-- ALTER TABLE ... ALTER COLUMN ... TYPE: PostgreSQL retypes the column in
-- place; SQLite has no such statement, so the table is rebuilt.

-- case: retyped column keeps its rows
-- setup:
CREATE TABLE a (id TEXT PRIMARY KEY, ref VARCHAR(20));
INSERT INTO a (id, ref) VALUES ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', 'x');
ALTER TABLE a ALTER COLUMN id TYPE uuid USING id::uuid;
-- query:
SELECT id, ref FROM a
-- expect:
6ba7b810-9dad-11d1-80b4-00c04fd430c8|x

-- case: retyped column behaves as the new type
-- setup:
CREATE TABLE a (id TEXT PRIMARY KEY, ref VARCHAR(20));
INSERT INTO a (id, ref) VALUES ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', 'x');
ALTER TABLE a ALTER COLUMN id TYPE uuid USING id::uuid;
-- query:
SELECT ref FROM a WHERE id = '6BA7B810-9DAD-11D1-80B4-00C04FD430C8'
-- expect:
x

-- case: retyped column validates as the new type
-- setup:
CREATE TABLE a (id TEXT PRIMARY KEY, ref VARCHAR(20));
ALTER TABLE a ALTER COLUMN id TYPE uuid USING id::uuid;
-- query:
INSERT INTO a (id, ref) VALUES ('not-a-uuid', 'y')
-- expect-error: 22P02

-- case: retyped column reports the new type in the catalog
-- setup:
CREATE TABLE a (id TEXT PRIMARY KEY, ref VARCHAR(20));
ALTER TABLE a ALTER COLUMN id SET DATA TYPE UUID;
-- query:
SELECT udt_name FROM information_schema.columns WHERE table_name = 'a' AND column_name = 'id'
-- expect:
uuid

-- case: using expression converts the values
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, n TEXT);
INSERT INTO t (id, n) VALUES (1, '41');
ALTER TABLE t ALTER COLUMN n TYPE INTEGER USING n::integer + 1;
-- query:
SELECT n FROM t
-- expect:
42

-- case: retype away from uuid drops its validation
-- setup:
CREATE TABLE t (id UUID PRIMARY KEY);
ALTER TABLE t ALTER COLUMN id TYPE TEXT;
INSERT INTO t (id) VALUES ('free text');
-- query:
SELECT id FROM t
-- expect:
free text

-- case: index survives the retype
-- setup:
CREATE TABLE a (id TEXT PRIMARY KEY, ref VARCHAR(20));
CREATE INDEX a_ref ON a (ref);
ALTER TABLE a ALTER COLUMN id TYPE uuid USING id::uuid;
-- query:
SELECT indexname FROM pg_indexes WHERE tablename = 'a' ORDER BY indexname
-- expect:
a_pkey
a_ref

-- case: unique constraint survives the retype
-- setup:
CREATE TABLE a (id TEXT PRIMARY KEY, ref VARCHAR(20) UNIQUE);
INSERT INTO a (id, ref) VALUES ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', 'x');
ALTER TABLE a ALTER COLUMN id TYPE uuid USING id::uuid;
-- query:
INSERT INTO a (id, ref) VALUES ('7ba7b810-9dad-11d1-80b4-00c04fd430c8', 'x')
-- expect-error: 23505

-- case: referencing table still joins after the parent is retyped
-- setup:
CREATE TABLE parent (id TEXT PRIMARY KEY, name TEXT);
CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id TEXT NOT NULL REFERENCES parent(id));
INSERT INTO parent (id, name) VALUES ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', 'p');
INSERT INTO child (id, parent_id) VALUES (1, '6ba7b810-9dad-11d1-80b4-00c04fd430c8');
ALTER TABLE parent ALTER COLUMN id TYPE uuid USING id::uuid;
ALTER TABLE child ALTER COLUMN parent_id TYPE uuid USING parent_id::uuid;
-- query:
SELECT p.name FROM child c JOIN parent p ON p.id = c.parent_id
-- expect:
p

-- case: view over the table still reads after the retype
-- setup:
CREATE TABLE a (id TEXT PRIMARY KEY, ref VARCHAR(20));
CREATE VIEW a_refs AS SELECT ref FROM a;
INSERT INTO a (id, ref) VALUES ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', 'x');
ALTER TABLE a ALTER COLUMN id TYPE uuid USING id::uuid;
-- query:
SELECT ref FROM a_refs
-- expect:
x

-- case: retype in a transaction rolls back with it
-- setup:
CREATE TABLE a (id TEXT PRIMARY KEY, ref VARCHAR(20));
INSERT INTO a (id, ref) VALUES ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', 'x');
BEGIN;
ALTER TABLE a ALTER COLUMN id TYPE uuid USING id::uuid;
ROLLBACK;
-- query:
SELECT udt_name FROM information_schema.columns WHERE table_name = 'a' AND column_name = 'id'
-- expect:
text

-- case: retype of an unknown table
-- query:
ALTER TABLE nowhere ALTER COLUMN id TYPE uuid
-- expect-error: 42P01

-- case: retype of an unknown column
-- setup:
CREATE TABLE a (id TEXT PRIMARY KEY);
-- query:
ALTER TABLE a ALTER COLUMN nope TYPE uuid
-- expect-error: 42703

-- case: several columns retyped in one statement
-- setup:
CREATE TABLE t (id TEXT PRIMARY KEY, batch_id TEXT NOT NULL, n TEXT);
INSERT INTO t (id, batch_id, n) VALUES ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', '7ba7b810-9dad-11d1-80b4-00c04fd430c8', '41');
ALTER TABLE t ALTER COLUMN id TYPE uuid USING id::uuid, ALTER COLUMN batch_id TYPE uuid USING batch_id::uuid, ALTER COLUMN n TYPE INTEGER USING n::integer + 1;
-- query:
SELECT udt_name FROM information_schema.columns WHERE table_name = 't' ORDER BY ordinal_position
-- expect:
uuid
uuid
int4

-- case: several columns retyped in one statement keep their rows
-- setup:
CREATE TABLE t (id TEXT PRIMARY KEY, batch_id TEXT NOT NULL, n TEXT);
INSERT INTO t (id, batch_id, n) VALUES ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', '7ba7b810-9dad-11d1-80b4-00c04fd430c8', '41');
ALTER TABLE t ALTER COLUMN id TYPE uuid USING id::uuid, ALTER COLUMN batch_id TYPE uuid USING batch_id::uuid, ALTER COLUMN n TYPE INTEGER USING n::integer + 1;
-- query:
SELECT n FROM t WHERE batch_id = '7BA7B810-9DAD-11D1-80B4-00C04FD430C8'
-- expect:
42
