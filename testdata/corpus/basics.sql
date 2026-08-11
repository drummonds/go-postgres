-- Basic CRUD and literal behavior.

-- case: select literal integer
-- query:
SELECT 1
-- expect:
1

-- case: arithmetic precedence
-- query:
SELECT 2 + 3 * 4
-- expect:
14

-- case: insert and select roundtrip
-- setup:
CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO items (id, name) VALUES (1, 'widget'), (2, 'gadget');
-- query:
SELECT id, name FROM items ORDER BY id
-- expect:
1|widget
2|gadget

-- case: update row
-- setup:
CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO items (id, name) VALUES (1, 'widget'), (2, 'gadget');
UPDATE items SET name = 'doohickey' WHERE id = 2;
-- query:
SELECT name FROM items WHERE id = 2
-- expect:
doohickey

-- case: delete row
-- setup:
CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO items (id, name) VALUES (1, 'widget'), (2, 'gadget');
DELETE FROM items WHERE id = 1;
-- query:
SELECT count(*) FROM items
-- expect:
1

-- case: select from empty table
-- setup:
CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT);
-- query:
SELECT id, name FROM items
-- expect:

-- case: insert returning
-- setup:
CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);
-- query:
INSERT INTO users (name) VALUES ('alice') RETURNING id, name
-- expect:
1|alice

-- case: on conflict do nothing
-- setup:
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT UNIQUE);
INSERT INTO users (id, name) VALUES (1, 'alice');
-- query:
INSERT INTO users (id, name) VALUES (2, 'alice') ON CONFLICT DO NOTHING RETURNING id
-- expect:
