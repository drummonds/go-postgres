-- PG :: cast syntax.

-- case: cast text to integer
-- query:
SELECT '42'::INTEGER
-- expect:
42

-- case: cast integer to text
-- query:
SELECT 42::TEXT
-- expect:
42

-- case: cast to boolean
-- query:
SELECT 1::BOOLEAN
-- expect:
1

-- case: cast to numeric preserves precision
-- query:
SELECT CAST('123456789.123456789' AS NUMERIC)
-- expect:
123456789.123456789

-- case: cast text to real
-- query:
SELECT '3.5'::REAL
-- expect:
3.5

-- case: cast of a qualified column
-- setup:
CREATE TABLE t (id UUID PRIMARY KEY, n INTEGER);
INSERT INTO t (id, n) VALUES ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', 1);
-- query:
SELECT ca.id::text || '!' FROM t ca
-- expect:
6ba7b810-9dad-11d1-80b4-00c04fd430c8!

-- case: cast of a qualified column in a comparison
-- setup:
CREATE TABLE t (id UUID PRIMARY KEY, n INTEGER);
CREATE TABLE s (owner VARCHAR(100));
INSERT INTO t (id, n) VALUES ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', 1);
INSERT INTO s (owner) VALUES ('6ba7b810-9dad-11d1-80b4-00c04fd430c8');
-- query:
SELECT ca.n FROM t ca JOIN s ON s.owner = ca.id::text
-- expect:
1
