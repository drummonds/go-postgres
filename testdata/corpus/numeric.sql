-- NUMERIC arithmetic: an expression with a ::numeric operand is exact and
-- carries PG's result scale (add/sub: the larger scale; mul: the sum;
-- div: at least 16 significant digits; round(x, n): exactly n).

-- case: integer division under a numeric cast is exact
-- query:
SELECT 7::numeric / 3
-- expect:
2.3333333333333333

-- case: round fixes the scale
-- query:
SELECT round(7::numeric / 3, 7)
-- expect:
2.3333333

-- case: accrual numerator over its denominator
-- query:
SELECT round(150000000::numeric / 3650000, 7)
-- expect:
41.0958904

-- case: multiplying integers under a cast keeps scale zero
-- query:
SELECT 1000000 * 150::numeric
-- expect:
150000000

-- case: multiplication adds the scales
-- query:
SELECT '2.50'::numeric * 4
-- expect:
10.00

-- case: addition is exact
-- query:
SELECT '0.1'::numeric + '0.2'::numeric
-- expect:
0.3

-- case: subtraction takes the larger scale
-- query:
SELECT 1 - '0.3'::numeric
-- expect:
0.7

-- case: cast with a declared scale rounds to it
-- query:
SELECT 7::numeric(10,2)
-- expect:
7.00

-- case: round with no scale rounds half away from zero
-- query:
SELECT round('2.5'::numeric), round('-2.5'::numeric)
-- expect:
3|-3

-- case: precedence is kept
-- query:
SELECT 1 + '2'::numeric * 3
-- expect:
7

-- case: a parenthesised numeric expression is an operand
-- query:
SELECT ('1.5'::numeric + 1) * 2
-- expect:
5.0

-- case: comparison is numeric, not textual
-- query:
SELECT CASE WHEN '10'::numeric > '9'::numeric THEN 1 ELSE 0 END
-- expect:
1

-- case: a view publishes a numeric column computed from integers
-- setup:
CREATE TABLE accrual (account TEXT, numerator BIGINT, denominator BIGINT);
INSERT INTO accrual VALUES ('a', 150000000, 3650000), ('b', -150000000, 3650000);
CREATE VIEW positions AS SELECT account, round(numerator::numeric / denominator, 7) AS accrued FROM accrual;
-- query:
SELECT account, accrued FROM positions ORDER BY account
-- expect:
a|41.0958904
b|-41.0958904

-- case: null propagates
-- query:
SELECT NULL::numeric / 3
-- expect:
NULL
