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

-- Aggregates. A bare NUMERIC column is TEXT in SQLite, so SUM/AVG would
-- go through REAL and MIN/MAX would compare lexicographically. The driver
-- rewrites aggregates over a column declared NUMERIC, and the translator
-- rewrites aggregates over a ::numeric expression, to exact pg_numeric_*
-- aggregates. Integer columns are untouched.

-- case: sum over a bare NUMERIC column is exact
-- setup:
CREATE TABLE ledger (amount NUMERIC(20,7));
INSERT INTO ledger VALUES ('0.1'), ('0.2'), ('-500.5420417');
-- query:
SELECT sum(amount) FROM ledger
-- expect:
-500.2420417

-- case: sum over a cast expression is exact
-- setup:
CREATE TABLE ledger (amount NUMERIC(20,7));
INSERT INTO ledger VALUES ('0.1'), ('0.2'), ('-500.5420417');
-- query:
SELECT sum(amount::numeric) FROM ledger
-- expect:
-500.2420417

-- case: min and max over a bare NUMERIC column compare numerically
-- setup:
CREATE TABLE ledger (amount NUMERIC);
INSERT INTO ledger VALUES ('100.0'), ('99.9'), ('-7');
-- query:
SELECT min(amount), max(amount) FROM ledger
-- expect:
-7|100.0

-- case: avg over a bare NUMERIC column carries PG's division scale
-- setup:
CREATE TABLE ledger (amount NUMERIC);
INSERT INTO ledger VALUES ('100.0'), ('99.9'), ('-7');
-- query:
SELECT avg(amount) FROM ledger
-- expect:
64.3000000000000000

-- case: aggregates over a NUMERIC column skip nulls and are null when empty
-- setup:
CREATE TABLE ledger (amount NUMERIC);
INSERT INTO ledger VALUES ('1.5'), (NULL);
-- query:
SELECT sum(amount), (SELECT sum(amount) FROM ledger WHERE amount IS NULL) FROM ledger
-- expect:
1.5|NULL

-- case: sum over an integer column is unchanged
-- setup:
CREATE TABLE counts (n BIGINT);
INSERT INTO counts VALUES (1), (2);
-- query:
SELECT sum(n) FROM counts
-- expect:
3

-- case: a qualified NUMERIC column in a join is summed exactly
-- setup:
CREATE TABLE ledger (account TEXT, amount NUMERIC(20,7));
CREATE TABLE accounts (name TEXT, open BOOLEAN);
INSERT INTO ledger VALUES ('a', '0.1'), ('a', '0.2');
INSERT INTO accounts VALUES ('a', TRUE);
-- query:
SELECT sum(l.amount) FROM ledger AS l JOIN accounts a ON a.name = l.account
-- expect:
0.3

-- case: a view summing a bare NUMERIC column is exact
-- setup:
CREATE TABLE ledger (account TEXT, amount NUMERIC(20,7));
INSERT INTO ledger VALUES ('a', '0.1'), ('a', '0.2');
CREATE VIEW balances AS SELECT account, sum(amount) AS balance FROM ledger GROUP BY account;
-- query:
SELECT balance FROM balances
-- expect:
0.3
