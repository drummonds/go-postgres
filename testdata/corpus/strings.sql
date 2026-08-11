-- String functions, ILIKE, E-strings, dollar quoting.

-- case: ilike is case insensitive
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO t (id, name) VALUES (1, 'Foo'), (2, 'bar'), (3, 'FOOBAR');
-- query:
SELECT name FROM t WHERE name ILIKE '%foo%' ORDER BY id
-- expect:
Foo
FOOBAR

-- case: e string escape newline
-- query:
SELECT length(E'a\nb')
-- expect:
3

-- case: dollar quoted string
-- query:
SELECT $$it's a test$$
-- expect:
it's a test

-- case: tagged dollar quoted string
-- query:
SELECT $fn$body text$fn$
-- expect:
body text

-- case: left function
-- query:
SELECT left('hello', 3)
-- expect:
hel

-- case: right function
-- query:
SELECT right('hello', 3)
-- expect:
llo

-- case: split_part
-- query:
SELECT split_part('a,b,c', ',', 2)
-- expect:
b

-- case: md5
-- query:
SELECT md5('hello')
-- expect:
5d41402abc4b2a76b9719d911017c592

-- case: string_agg
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO t (id, name) VALUES (1, 'a'), (2, 'b');
-- query:
SELECT string_agg(name, ', ') FROM t
-- expect:
a, b

-- case: array_agg returns json array
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO t (id, name) VALUES (1, 'a'), (2, 'b');
-- query:
SELECT array_agg(name) FROM t
-- expect:
["a","b"]

-- case: concat operator
-- query:
SELECT 'foo' || 'bar'
-- expect:
foobar
