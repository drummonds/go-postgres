-- $n parameter placeholders.

-- case: select with one param
-- setup:
CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO items (id, name) VALUES (1, 'widget'), (2, 'gadget');
-- query:
SELECT name FROM items WHERE id = $1
-- params: 2
-- expect:
gadget

-- case: insert with params returning
-- setup:
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
-- query:
INSERT INTO users (id, name) VALUES ($1, $2) RETURNING id, name
-- params: 7|alice
-- expect:
7|alice

-- case: param inside cast expression
-- query:
SELECT $1::INTEGER + 1
-- params: 41
-- expect:
42

-- case: param reused within one statement binds once
-- setup:
CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, alias TEXT);
INSERT INTO items (id, name, alias) VALUES (1, 'widget', 'w'), (2, 'gadget', 'gadget');
-- query:
SELECT id FROM items WHERE name = $1 OR alias = $1 ORDER BY id
-- params: gadget
-- expect:
2
