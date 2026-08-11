-- PG regex operators and SIMILAR TO.

-- case: tilde case sensitive match
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO t (id, name) VALUES (1, 'Alice'), (2, 'Bob'), (3, 'alex');
-- query:
SELECT name FROM t WHERE name ~ '^Al' ORDER BY id
-- expect:
Alice

-- case: tilde star case insensitive match
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO t (id, name) VALUES (1, 'Alice'), (2, 'Bob'), (3, 'alex');
-- query:
SELECT name FROM t WHERE name ~* '^al' ORDER BY id
-- expect:
Alice
alex

-- case: negated tilde
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO t (id, name) VALUES (1, 'Alice'), (2, 'Bob'), (3, 'alex');
-- query:
SELECT name FROM t WHERE name !~ '^Al' ORDER BY id
-- expect:
Bob
alex

-- case: negated tilde star
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO t (id, name) VALUES (1, 'Alice'), (2, 'Bob'), (3, 'alex');
-- query:
SELECT name FROM t WHERE name !~* '^al' ORDER BY id
-- expect:
Bob

-- case: similar to
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO t (id, name) VALUES (1, 'foo1'), (2, 'bar2'), (3, 'baz3');
-- query:
SELECT name FROM t WHERE name SIMILAR TO '%(foo|bar)%' ORDER BY id
-- expect:
foo1
bar2

-- case: not similar to
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO t (id, name) VALUES (1, 'foo1'), (2, 'bar2'), (3, 'baz3');
-- query:
SELECT name FROM t WHERE name NOT SIMILAR TO '%(foo|bar)%' ORDER BY id
-- expect:
baz3
