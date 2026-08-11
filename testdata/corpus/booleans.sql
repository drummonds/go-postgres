-- Boolean literals and predicates (stored as 0/1 integers).

-- case: true literal
-- query:
SELECT TRUE
-- expect:
1

-- case: false literal
-- query:
SELECT FALSE
-- expect:
0

-- case: where equals true
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, active BOOLEAN);
INSERT INTO t (id, active) VALUES (1, TRUE), (2, FALSE), (3, TRUE);
-- query:
SELECT id FROM t WHERE active = TRUE ORDER BY id
-- expect:
1
3

-- case: is true
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, active BOOLEAN);
INSERT INTO t (id, active) VALUES (1, TRUE), (2, FALSE);
-- query:
SELECT id FROM t WHERE active IS TRUE
-- expect:
1

-- case: is false
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, active BOOLEAN);
INSERT INTO t (id, active) VALUES (1, TRUE), (2, FALSE);
-- query:
SELECT id FROM t WHERE active IS FALSE
-- expect:
2

-- case: is not true
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, active BOOLEAN);
INSERT INTO t (id, active) VALUES (1, TRUE), (2, FALSE);
-- query:
SELECT id FROM t WHERE active IS NOT TRUE
-- expect:
2
