-- ORDER BY ... NULLS FIRST / NULLS LAST.

-- case: asc nulls first
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO t (id, name) VALUES (1, 'b'), (2, NULL), (3, 'a');
-- query:
SELECT name FROM t ORDER BY name ASC NULLS FIRST
-- expect:
NULL
a
b

-- case: asc nulls last
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO t (id, name) VALUES (1, 'b'), (2, NULL), (3, 'a');
-- query:
SELECT name FROM t ORDER BY name ASC NULLS LAST
-- expect:
a
b
NULL

-- case: desc nulls first
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO t (id, name) VALUES (1, 'b'), (2, NULL), (3, 'a');
-- query:
SELECT name FROM t ORDER BY name DESC NULLS FIRST
-- expect:
NULL
b
a

-- case: bare nulls last
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO t (id, name) VALUES (1, 'b'), (2, NULL), (3, 'a');
-- query:
SELECT name FROM t ORDER BY name NULLS LAST
-- expect:
a
b
NULL
