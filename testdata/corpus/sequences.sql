-- CREATE SEQUENCE / nextval / currval emulation.

-- case: nextval starts at one
-- setup:
CREATE SEQUENCE s;
-- query:
SELECT nextval('s')
-- expect:
1

-- case: nextval increments
-- setup:
CREATE SEQUENCE s;
SELECT nextval('s');
-- query:
SELECT nextval('s')
-- expect:
2

-- case: currval returns last value
-- setup:
CREATE SEQUENCE s;
SELECT nextval('s');
SELECT nextval('s');
-- query:
SELECT currval('s')
-- expect:
2

-- case: start with
-- setup:
CREATE SEQUENCE s START WITH 100;
-- query:
SELECT nextval('s')
-- expect:
100
