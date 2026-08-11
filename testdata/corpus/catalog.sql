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
