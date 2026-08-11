-- Misc PG function shims.

-- case: gen_random_uuid format
-- query:
SELECT gen_random_uuid()
-- expect-match:
[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}

-- case: pg_typeof integer
-- query:
SELECT pg_typeof(42)
-- expect:
integer

-- case: coalesce
-- query:
SELECT coalesce(NULL, 'x')
-- expect:
x

-- case: nullif equal values
-- query:
SELECT nullif(1, 1)
-- expect:
NULL
