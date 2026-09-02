-- PG catalog compatibility views.

-- case: information_schema tables lists user tables
-- setup:
CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);
CREATE TABLE orders (id SERIAL PRIMARY KEY, user_id INTEGER);
-- query:
SELECT table_name FROM information_schema.tables ORDER BY table_name
-- expect:
orders
users

-- case: information_schema columns in order
-- setup:
CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);
-- query:
SELECT column_name FROM information_schema.columns WHERE table_name = 'users' ORDER BY ordinal_position
-- expect:
id
name

-- case: pg_indexes lists index
-- setup:
CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);
CREATE INDEX idx_users_name ON users (name);
-- query:
SELECT indexname FROM pg_indexes WHERE tablename = 'users'
-- expect:
idx_users_name

-- case: pg_tables lists tables but not views
-- setup:
CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);
CREATE TABLE orders (id SERIAL PRIMARY KEY, user_id INTEGER);
CREATE VIEW v_users AS SELECT id, name FROM users;
-- query:
SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename
-- expect:
orders
users

-- case: pg_views lists views
-- setup:
CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);
CREATE VIEW v_users AS SELECT id, name FROM users;
-- query:
SELECT viewname FROM pg_views WHERE schemaname = 'public'
-- expect:
v_users

-- case: columns joined to primary key flag with reused param
-- setup:
CREATE TABLE orders (id SERIAL PRIMARY KEY, user_id INTEGER NOT NULL, note TEXT);
-- query:
SELECT c.column_name, c.is_nullable, CASE WHEN pk.column_name IS NULL THEN 'no' ELSE 'yes' END
FROM information_schema.columns c
LEFT JOIN (SELECT kcu.column_name
    FROM information_schema.table_constraints tc
    JOIN information_schema.key_column_usage kcu
      ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
    WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = 'public' AND tc.table_name = $1
) pk ON pk.column_name = c.column_name
WHERE c.table_schema = 'public' AND c.table_name = $1
ORDER BY c.ordinal_position
-- params: orders
-- expect:
id|NO|yes
user_id|NO|no
note|YES|no

-- case: current_schema
-- query:
SELECT current_schema()
-- expect:
public

-- case: current_database
-- query:
SELECT current_database()
-- expect:
main
