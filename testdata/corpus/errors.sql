-- PG-style SQLSTATE error codes.

-- case: unique violation
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT UNIQUE);
INSERT INTO t (id, name) VALUES (1, 'alice');
-- query:
INSERT INTO t (id, name) VALUES (2, 'alice')
-- expect-error: 23505

-- case: not null violation
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
-- query:
INSERT INTO t (id, name) VALUES (1, NULL)
-- expect-error: 23502

-- case: undefined table
-- query:
SELECT * FROM nonexistent_table
-- expect-error: 42P01
