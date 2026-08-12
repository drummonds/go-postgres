-- generate_series table function.

-- case: simple series
-- query:
SELECT * FROM generate_series(1, 5)
-- expect:
1
2
3
4
5

-- case: series with step
-- query:
SELECT * FROM generate_series(0, 10, 2)
-- expect:
0
2
4
6
8
10

-- case: series with alias
-- query:
SELECT s FROM generate_series(1, 3) AS s
-- expect:
1
2
3
