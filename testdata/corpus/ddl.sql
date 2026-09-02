-- DDL translation behavior: PG types and defaults on a SQLite engine.

-- case: serial primary key autoincrements
-- setup:
CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);
INSERT INTO users (name) VALUES ('alice');
INSERT INTO users (name) VALUES ('bob');
-- query:
SELECT id, name FROM users ORDER BY id
-- expect:
1|alice
2|bob

-- case: bigserial autoincrements
-- setup:
CREATE TABLE events (id BIGSERIAL PRIMARY KEY, kind TEXT);
INSERT INTO events (kind) VALUES ('start');
INSERT INTO events (kind) VALUES ('stop');
-- query:
SELECT id, kind FROM events ORDER BY id
-- expect:
1|start
2|stop

-- case: varchar column stores text
-- setup:
CREATE TABLE t (name VARCHAR(100));
INSERT INTO t (name) VALUES ('hello');
-- query:
SELECT name FROM t
-- expect:
hello

-- case: character varying column stores text
-- setup:
CREATE TABLE t (name CHARACTER VARYING(255));
INSERT INTO t (name) VALUES ('hello');
-- query:
SELECT name FROM t
-- expect:
hello

-- case: boolean column default true
-- setup:
CREATE TABLE flags (id SERIAL PRIMARY KEY, active BOOLEAN DEFAULT TRUE);
INSERT INTO flags (id) VALUES (1);
-- query:
SELECT active FROM flags
-- expect:
1

-- case: timestamp default now is populated
-- setup:
CREATE TABLE t (id SERIAL PRIMARY KEY, created_at TIMESTAMP DEFAULT NOW());
INSERT INTO t (id) VALUES (1);
-- query:
SELECT created_at FROM t
-- expect-match:
\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}.*

-- case: uuid default gen_random_uuid is populated
-- setup:
CREATE TABLE t (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), ts TIMESTAMP NOT NULL);
INSERT INTO t (ts) VALUES ('2026-02-02 10:00:00');
-- query:
SELECT id FROM t
-- expect-match:
[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}

-- case: timestamptz default now with not null is populated
-- setup:
CREATE TABLE t (id SERIAL PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT now());
INSERT INTO t (id) VALUES (1);
-- query:
SELECT created_at FROM t
-- expect-match:
\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}.*

-- case: numeric column preserves precision
-- setup:
CREATE TABLE prices (id INTEGER PRIMARY KEY, price NUMERIC(20,10));
INSERT INTO prices (id, price) VALUES (1, '123456789.1234567890');
-- query:
SELECT price FROM prices
-- expect:
123456789.1234567890

-- case: uuid column roundtrip
-- setup:
CREATE TABLE t (id UUID);
INSERT INTO t (id) VALUES ('123e4567-e89b-12d3-a456-426614174000');
-- query:
SELECT id FROM t
-- expect:
123e4567-e89b-12d3-a456-426614174000

-- case: jsonb column roundtrip
-- setup:
CREATE TABLE t (meta JSONB);
INSERT INTO t (meta) VALUES ('{"a": 1}');
-- query:
SELECT meta FROM t
-- expect:
{"a": 1}

-- case: alter table add column if not exists
-- setup:
CREATE TABLE t (id INTEGER PRIMARY KEY);
ALTER TABLE t ADD COLUMN IF NOT EXISTS email TEXT;
INSERT INTO t (id, email) VALUES (1, 'x@y.z');
-- query:
SELECT email FROM t
-- expect:
x@y.z
