-- UUID columns: stored as text, but they compare and validate as
-- PostgreSQL's uuid type does.

-- case: uuid column matches a lookup in another case
-- setup:
CREATE TABLE t (id UUID PRIMARY KEY, n INTEGER);
INSERT INTO t (id, n) VALUES ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', 1);
-- query:
SELECT n FROM t WHERE id = '6BA7B810-9DAD-11D1-80B4-00C04FD430C8'
-- expect:
1

-- case: uuid column rejects a value that is not a uuid
-- setup:
CREATE TABLE t (id UUID PRIMARY KEY, n INTEGER);
-- query:
INSERT INTO t (id, n) VALUES ('not-a-uuid', 1)
-- expect-error: 22P02

-- case: uuid column rejects a uuid without hyphens
-- setup:
CREATE TABLE t (id UUID PRIMARY KEY, n INTEGER);
-- query:
INSERT INTO t (id, n) VALUES ('6ba7b8109dad11d180b400c04fd430c8', 1)
-- expect-error: 22P02

-- case: nullable uuid column accepts null
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, parent UUID);
INSERT INTO t (id, parent) VALUES (1, NULL);
-- query:
SELECT parent FROM t WHERE id = 1
-- expect:
NULL

-- case: uuid join matches across case
-- setup:
CREATE TABLE parent (id UUID PRIMARY KEY, name TEXT);
CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id UUID NOT NULL REFERENCES parent(id));
INSERT INTO parent (id, name) VALUES ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', 'p');
INSERT INTO child (id, parent_id) VALUES (1, '6BA7B810-9DAD-11D1-80B4-00C04FD430C8');
-- query:
SELECT p.name FROM child c JOIN parent p ON p.id = c.parent_id
-- expect:
p

-- case: uuid column reports uuid in the catalog
-- setup:
CREATE TABLE t (id UUID PRIMARY KEY, n INTEGER);
-- query:
SELECT udt_name FROM information_schema.columns WHERE table_name = 't' AND column_name = 'id'
-- expect:
uuid

-- case: uuid column added by alter table validates
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY);
ALTER TABLE t ADD COLUMN ref UUID;
-- query:
INSERT INTO t (id, ref) VALUES (1, 'not-a-uuid')
-- expect-error: 22P02

-- case: uuid default is populated and reads back
-- setup:
CREATE TABLE t (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), n INTEGER);
INSERT INTO t (n) VALUES (1);
-- query:
SELECT length(id::text) FROM t
-- expect:
36
