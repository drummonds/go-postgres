-- ALTER TABLE ... DROP CONSTRAINT and ADD CONSTRAINT ... FOREIGN KEY:
-- SQLite has neither, so the table is rebuilt. Foreign key constraints
-- are named as PostgreSQL names an unnamed one, <table>_<column>_fkey,
-- so a name read from the catalog can be dropped on either driver.

-- case: foreign key constraint is named as PostgreSQL names it
-- setup:
CREATE TABLE parent (id TEXT PRIMARY KEY);
CREATE TABLE child (id INTEGER PRIMARY KEY, a_id TEXT REFERENCES parent(id), b_id TEXT REFERENCES parent(id));
-- query:
SELECT tc.constraint_name, kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu ON kcu.constraint_name = tc.constraint_name
WHERE tc.table_name = 'child' AND tc.constraint_type = 'FOREIGN KEY'
ORDER BY tc.constraint_name
-- expect:
child_a_id_fkey|a_id
child_b_id_fkey|b_id

-- case: dropped foreign key no longer constrains
-- setup:
CREATE TABLE parent (id TEXT PRIMARY KEY);
CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id TEXT REFERENCES parent(id));
ALTER TABLE child DROP CONSTRAINT child_parent_id_fkey;
INSERT INTO child (id, parent_id) VALUES (1, 'orphan');
-- query:
SELECT parent_id FROM child
-- expect:
orphan

-- case: dropped foreign key leaves the catalog
-- setup:
CREATE TABLE parent (id TEXT PRIMARY KEY);
CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id TEXT REFERENCES parent(id));
ALTER TABLE child DROP CONSTRAINT child_parent_id_fkey;
-- query:
SELECT count(*) FROM information_schema.table_constraints WHERE table_name = 'child' AND constraint_type = 'FOREIGN KEY'
-- expect:
0

-- case: dropping one of two foreign keys keeps the other
-- setup:
CREATE TABLE parent (id TEXT PRIMARY KEY);
CREATE TABLE child (id INTEGER PRIMARY KEY, a_id TEXT REFERENCES parent(id), b_id TEXT REFERENCES parent(id));
ALTER TABLE child DROP CONSTRAINT child_a_id_fkey;
-- query:
SELECT constraint_name FROM information_schema.table_constraints WHERE table_name = 'child' AND constraint_type = 'FOREIGN KEY'
-- expect:
child_b_id_fkey

-- case: table-level named foreign key is dropped by its name
-- setup:
CREATE TABLE parent (id TEXT PRIMARY KEY);
CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id TEXT, CONSTRAINT child_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES parent(id));
ALTER TABLE child DROP CONSTRAINT child_parent_id_fkey;
INSERT INTO child (id, parent_id) VALUES (1, 'orphan');
-- query:
SELECT parent_id FROM child
-- expect:
orphan

-- case: added foreign key constrains
-- setup:
CREATE TABLE parent (id TEXT PRIMARY KEY);
CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id TEXT);
ALTER TABLE child ADD CONSTRAINT child_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES parent (id);
-- query:
INSERT INTO child (id, parent_id) VALUES (1, 'orphan')
-- expect-error: 23503

-- case: added foreign key is in the catalog
-- setup:
CREATE TABLE parent (id TEXT PRIMARY KEY);
CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id TEXT);
ALTER TABLE child ADD CONSTRAINT child_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES parent (id);
-- query:
SELECT constraint_name FROM information_schema.table_constraints WHERE table_name = 'child' AND constraint_type = 'FOREIGN KEY'
-- expect:
child_parent_id_fkey

-- case: added foreign key keeps the rows
-- setup:
CREATE TABLE parent (id TEXT PRIMARY KEY);
CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id TEXT);
INSERT INTO parent (id) VALUES ('p');
INSERT INTO child (id, parent_id) VALUES (1, 'p');
ALTER TABLE child ADD CONSTRAINT child_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES parent (id);
-- query:
SELECT parent_id FROM child
-- expect:
p

-- case: retype a referenced key by dropping and re-adding the foreign key
-- setup:
CREATE TABLE parent (id TEXT PRIMARY KEY, name TEXT);
CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id TEXT NOT NULL REFERENCES parent(id));
INSERT INTO parent (id, name) VALUES ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', 'p');
INSERT INTO child (id, parent_id) VALUES (1, '6ba7b810-9dad-11d1-80b4-00c04fd430c8');
ALTER TABLE child DROP CONSTRAINT child_parent_id_fkey;
ALTER TABLE parent ALTER COLUMN id TYPE uuid USING id::uuid;
ALTER TABLE child ALTER COLUMN parent_id TYPE uuid USING parent_id::uuid;
ALTER TABLE child ADD CONSTRAINT child_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES parent (id);
-- query:
INSERT INTO child (id, parent_id) VALUES (2, '7ba7b810-9dad-11d1-80b4-00c04fd430c8')
-- expect-error: 23503

-- case: drop of an unknown constraint
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY);
-- query:
ALTER TABLE t DROP CONSTRAINT nope
-- expect-error: 42704

-- case: drop if exists of an unknown constraint is quiet
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, n INTEGER);
ALTER TABLE t DROP CONSTRAINT IF EXISTS nope;
INSERT INTO t (id, n) VALUES (1, 1);
-- query:
SELECT n FROM t
-- expect:
1
