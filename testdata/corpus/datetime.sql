-- Date/time functions, EXTRACT, date_trunc, to_char, intervals.

-- case: extract year
-- query:
SELECT EXTRACT(year FROM '2024-03-15')
-- expect:
2024

-- case: extract month
-- query:
SELECT EXTRACT(month FROM '2024-03-15')
-- expect:
3

-- case: extract day
-- query:
SELECT EXTRACT(day FROM '2024-03-15')
-- expect:
15

-- case: date_trunc day
-- query:
SELECT date_trunc('day', '2024-03-15 14:30:00')
-- expect:
2024-03-15

-- case: date_trunc month
-- query:
SELECT date_trunc('month', '2024-03-15 14:30:00')
-- expect:
2024-03-01

-- case: date_trunc year
-- query:
SELECT date_trunc('year', '2024-03-15 14:30:00')
-- expect:
2024-01-01

-- case: date_trunc hour
-- query:
SELECT date_trunc('hour', '2024-03-15 14:30:00')
-- expect:
2024-03-15 14:00:00

-- case: to_char date pattern
-- query:
SELECT to_char('2024-03-15 14:30:00', 'YYYY-MM-DD')
-- expect:
2024-03-15

-- case: to_char time pattern
-- query:
SELECT to_char('2024-03-15 14:30:00', 'HH24:MI:SS')
-- expect:
14:30:00

-- case: to_char month name
-- query:
SELECT to_char('2024-03-15 14:30:00', 'Mon DD, YYYY')
-- expect:
Mar 15, 2024

-- case: interval add day
-- query:
SELECT '2024-01-15 10:00:00' + INTERVAL '1 day'
-- expect:
2024-01-16 10:00:00

-- case: interval subtract hours
-- query:
SELECT '2024-01-15 10:00:00' - INTERVAL '2 hours'
-- expect:
2024-01-15 08:00:00

-- case: interval add minutes
-- query:
SELECT '2024-01-15 10:00:00' + INTERVAL '30 minutes'
-- expect:
2024-01-15 10:30:00

-- case: interval sql standard syntax
-- query:
SELECT '2024-01-15 10:00:00' + INTERVAL '1' DAY
-- expect:
2024-01-16 10:00:00

-- case: now returns a timestamp
-- query:
SELECT NOW()
-- expect-match:
\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}.*

-- case: current_date returns a date
-- query:
SELECT CURRENT_DATE
-- expect-match:
\d{4}-\d{2}-\d{2}
