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
