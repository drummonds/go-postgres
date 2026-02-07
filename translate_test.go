package pglike

import (
	"testing"
)

func TestTranslateDDL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "SERIAL PRIMARY KEY",
			input: "CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT)",
			want:  "CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT)",
		},
		{
			name:  "SERIAL without PRIMARY KEY",
			input: "CREATE TABLE users (id SERIAL, name TEXT)",
			want:  "CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT)",
		},
		{
			name:  "BIGSERIAL",
			input: "CREATE TABLE t (id BIGSERIAL PRIMARY KEY)",
			want:  "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT)",
		},
		{
			name:  "SERIAL NOT NULL PRIMARY KEY",
			input: "CREATE TABLE t (id SERIAL NOT NULL PRIMARY KEY, name TEXT)",
			want:  "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, name TEXT)",
		},
		{
			name:  "SERIAL UNIQUE PRIMARY KEY",
			input: "CREATE TABLE t (id SERIAL UNIQUE PRIMARY KEY, name TEXT)",
			want:  "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT UNIQUE, name TEXT)",
		},
		{
			name:  "SERIAL CONSTRAINT pk PRIMARY KEY",
			input: "CREATE TABLE t (id SERIAL CONSTRAINT pk PRIMARY KEY)",
			want:  "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT)",
		},
		{
			name:  "SMALLSERIAL NOT NULL PRIMARY KEY",
			input: "CREATE TABLE t (id SMALLSERIAL NOT NULL PRIMARY KEY)",
			want:  "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL)",
		},
		{
			name:  "VARCHAR(n) to TEXT",
			input: "CREATE TABLE t (name VARCHAR(100))",
			want:  "CREATE TABLE t (name TEXT)",
		},
		{
			name:  "CHARACTER VARYING(n) to TEXT",
			input: "CREATE TABLE t (name CHARACTER VARYING(255))",
			want:  "CREATE TABLE t (name TEXT)",
		},
		{
			name:  "BOOLEAN to INTEGER",
			input: "CREATE TABLE t (active BOOLEAN)",
			want:  "CREATE TABLE t (active INTEGER)",
		},
		{
			name:  "TIMESTAMP WITH TIME ZONE",
			input: "CREATE TABLE t (created_at TIMESTAMP WITH TIME ZONE)",
			want:  "CREATE TABLE t (created_at TEXT)",
		},
		{
			name:  "TIMESTAMPTZ",
			input: "CREATE TABLE t (ts TIMESTAMPTZ)",
			want:  "CREATE TABLE t (ts TEXT)",
		},
		{
			name:  "UUID",
			input: "CREATE TABLE t (id UUID)",
			want:  "CREATE TABLE t (id TEXT)",
		},
		{
			name:  "BYTEA",
			input: "CREATE TABLE t (data BYTEA)",
			want:  "CREATE TABLE t (data BLOB)",
		},
		{
			name:  "JSONB",
			input: "CREATE TABLE t (meta JSONB)",
			want:  "CREATE TABLE t (meta TEXT)",
		},
		{
			name:  "DOUBLE PRECISION",
			input: "CREATE TABLE t (val DOUBLE PRECISION)",
			want:  "CREATE TABLE t (val REAL)",
		},
		{
			name:  "NUMERIC(10,2)",
			input: "CREATE TABLE t (price NUMERIC(10,2))",
			want:  "CREATE TABLE t (price REAL)",
		},
		{
			name:  "SMALLINT",
			input: "CREATE TABLE t (n SMALLINT)",
			want:  "CREATE TABLE t (n INTEGER)",
		},
		{
			name:  "BIGINT",
			input: "CREATE TABLE t (n BIGINT)",
			want:  "CREATE TABLE t (n INTEGER)",
		},
		{
			name:  "DEFAULT NOW()",
			input: "CREATE TABLE t (created_at TIMESTAMP DEFAULT NOW())",
			want:  "CREATE TABLE t (created_at TEXT DEFAULT (datetime('now')))",
		},
		{
			name:  "complex table",
			input: "CREATE TABLE users (id SERIAL PRIMARY KEY, name VARCHAR(100) NOT NULL, email VARCHAR(255) UNIQUE, active BOOLEAN DEFAULT TRUE, created_at TIMESTAMP DEFAULT NOW())",
			want:  "CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, email TEXT UNIQUE, active INTEGER DEFAULT 1, created_at TEXT DEFAULT (datetime('now')))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Translate(tt.input)
			if err != nil {
				t.Fatalf("Translate() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate()\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestTranslateExpressions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "::INTEGER cast",
			input: "SELECT '42'::INTEGER",
			want:  "SELECT CAST('42' AS INTEGER)",
		},
		{
			name:  "::TEXT cast",
			input: "SELECT 42::TEXT",
			want:  "SELECT CAST(42 AS TEXT)",
		},
		{
			name:  "::BOOLEAN cast",
			input: "SELECT 1::BOOLEAN",
			want:  "SELECT CAST(1 AS INTEGER)",
		},
		{
			name:  "ILIKE to LIKE",
			input: "SELECT * FROM t WHERE name ILIKE '%foo%'",
			want:  "SELECT * FROM t WHERE name LIKE '%foo%'",
		},
		{
			name:  "TRUE literal",
			input: "SELECT * FROM t WHERE active = TRUE",
			want:  "SELECT * FROM t WHERE active = 1",
		},
		{
			name:  "FALSE literal",
			input: "SELECT * FROM t WHERE active = FALSE",
			want:  "SELECT * FROM t WHERE active = 0",
		},
		{
			name:  "IS TRUE",
			input: "SELECT * FROM t WHERE active IS TRUE",
			want:  "SELECT * FROM t WHERE active = 1",
		},
		{
			name:  "IS FALSE",
			input: "SELECT * FROM t WHERE active IS FALSE",
			want:  "SELECT * FROM t WHERE active = 0",
		},
		{
			name:  "IS NOT TRUE",
			input: "SELECT * FROM t WHERE active IS NOT TRUE",
			want:  "SELECT * FROM t WHERE active != 1",
		},
		{
			name:  "IS NOT FALSE",
			input: "SELECT * FROM t WHERE active IS NOT FALSE",
			want:  "SELECT * FROM t WHERE active != 0",
		},
		{
			name:  "E string with newline",
			input: `SELECT E'hello\nworld'`,
			want:  "SELECT 'hello\nworld'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Translate(tt.input)
			if err != nil {
				t.Fatalf("Translate() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate()\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestTranslateFunctions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "NOW()",
			input: "SELECT NOW()",
			want:  "SELECT datetime('now')",
		},
		{
			name:  "CURRENT_DATE",
			input: "SELECT CURRENT_DATE",
			want:  "SELECT date('now')",
		},
		{
			name:  "CURRENT_TIMESTAMP",
			input: "SELECT CURRENT_TIMESTAMP",
			want:  "SELECT datetime('now')",
		},
		{
			name:  "EXTRACT year",
			input: "SELECT EXTRACT(year FROM created_at) FROM t",
			want:  "SELECT CAST(strftime('%Y', created_at) AS INTEGER) FROM t",
		},
		{
			name:  "EXTRACT month",
			input: "SELECT EXTRACT(month FROM created_at) FROM t",
			want:  "SELECT CAST(strftime('%m', created_at) AS INTEGER) FROM t",
		},
		{
			name:  "EXTRACT day",
			input: "SELECT EXTRACT(day FROM ts) FROM t",
			want:  "SELECT CAST(strftime('%d', ts) AS INTEGER) FROM t",
		},
		{
			name:  "date_trunc day",
			input: "SELECT date_trunc('day', created_at) FROM t",
			want:  "SELECT date(created_at) FROM t",
		},
		{
			name:  "date_trunc month",
			input: "SELECT date_trunc('month', created_at) FROM t",
			want:  "SELECT strftime('%Y-%m-01', created_at) FROM t",
		},
		{
			name:  "date_trunc year",
			input: "SELECT date_trunc('year', created_at) FROM t",
			want:  "SELECT strftime('%Y-01-01', created_at) FROM t",
		},
		{
			name:  "date_trunc hour",
			input: "SELECT date_trunc('hour', ts) FROM t",
			want:  "SELECT strftime('%Y-%m-%d %H:00:00', ts) FROM t",
		},
		{
			name:  "left(str, n)",
			input: "SELECT left(name, 3) FROM t",
			want:  "SELECT substr(name, 1, 3) FROM t",
		},
		{
			name:  "right(str, n)",
			input: "SELECT right(name, 3) FROM t",
			want:  "SELECT substr(name, -3) FROM t",
		},
		{
			name:  "string_agg",
			input: "SELECT string_agg(name, ', ') FROM t",
			want:  "SELECT group_concat(name, ', ') FROM t",
		},
		{
			name:  "array_agg",
			input: "SELECT array_agg(name) FROM t",
			want:  "SELECT json_group_array(name) FROM t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Translate(tt.input)
			if err != nil {
				t.Fatalf("Translate() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate()\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestTranslateParams(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "$1 param",
			input: "SELECT * FROM t WHERE id = $1",
			want:  "SELECT * FROM t WHERE id = ?",
		},
		{
			name:  "multiple params",
			input: "INSERT INTO t (a, b) VALUES ($1, $2)",
			want:  "INSERT INTO t (a, b) VALUES (?, ?)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Translate(tt.input)
			if err != nil {
				t.Fatalf("Translate() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate()\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestTranslateRegexOps(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "~ case sensitive match",
			input: "SELECT * FROM t WHERE name ~ '^foo'",
			want:  "SELECT * FROM t WHERE pg_regex_match(name, '^foo', 0)",
		},
		{
			name:  "~* case insensitive match",
			input: "SELECT * FROM t WHERE name ~* '^foo'",
			want:  "SELECT * FROM t WHERE pg_regex_match(name, '^foo', 1)",
		},
		{
			name:  "!~ negated case sensitive",
			input: "SELECT * FROM t WHERE name !~ '^foo'",
			want:  "SELECT * FROM t WHERE NOT pg_regex_match(name, '^foo', 0)",
		},
		{
			name:  "!~* negated case insensitive",
			input: "SELECT * FROM t WHERE name !~* '^foo'",
			want:  "SELECT * FROM t WHERE NOT pg_regex_match(name, '^foo', 1)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Translate(tt.input)
			if err != nil {
				t.Fatalf("Translate() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate()\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestTranslateSequenceDDL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "CREATE SEQUENCE basic",
			input: "CREATE SEQUENCE my_seq",
			want:  "INSERT OR IGNORE INTO _sequences (name, current_value, increment) VALUES ('my_seq', 0, 1)",
		},
		{
			name:  "CREATE SEQUENCE with INCREMENT",
			input: "CREATE SEQUENCE my_seq INCREMENT BY 5",
			want:  "INSERT OR IGNORE INTO _sequences (name, current_value, increment) VALUES ('my_seq', 0, 5)",
		},
		{
			name:  "CREATE SEQUENCE with START",
			input: "CREATE SEQUENCE my_seq START WITH 100",
			want:  "INSERT OR IGNORE INTO _sequences (name, current_value, increment) VALUES ('my_seq', 99, 1)",
		},
		{
			name:  "DROP SEQUENCE",
			input: "DROP SEQUENCE my_seq",
			want:  "DELETE FROM _sequences WHERE name = 'my_seq'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Translate(tt.input)
			if err != nil {
				t.Fatalf("Translate() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate()\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestTranslateGenerateSeries(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple generate_series",
			input: "SELECT * FROM generate_series(1, 5)",
			want:  "WITH RECURSIVE _gs(value) AS (SELECT 1 UNION ALL SELECT value + 1 FROM _gs WHERE value + 1 <= 5) SELECT * FROM _gs",
		},
		{
			name:  "generate_series with step",
			input: "SELECT * FROM generate_series(0, 10, 2)",
			want:  "WITH RECURSIVE _gs(value) AS (SELECT 0 UNION ALL SELECT value + 2 FROM _gs WHERE value + 2 <= 10) SELECT * FROM _gs",
		},
		{
			name:  "generate_series with alias",
			input: "SELECT s FROM generate_series(1, 3) AS s",
			want:  "WITH RECURSIVE _gs(value) AS (SELECT 1 UNION ALL SELECT value + 1 FROM _gs WHERE value + 1 <= 3) SELECT s FROM _gs AS s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Translate(tt.input)
			if err != nil {
				t.Fatalf("Translate() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate()\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestTranslateInterval(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "NOW() + INTERVAL '1 day'",
			input: "SELECT NOW() + INTERVAL '1 day'",
			want:  "SELECT datetime(datetime('now'), '+1 day')",
		},
		// Note: translateInterval runs before translateFunctions in the pipeline,
		// so NOW() inside datetime() gets translated by translateNow afterward.
		{
			name:  "ts - INTERVAL '2 hours'",
			input: "SELECT ts - INTERVAL '2 hours' FROM t",
			want:  "SELECT datetime(ts, '-2 hours') FROM t",
		},
		{
			name:  "INTERVAL '30 minutes'",
			input: "SELECT ts + INTERVAL '30 minutes' FROM t",
			want:  "SELECT datetime(ts, '+30 minutes') FROM t",
		},
		{
			name:  "INTERVAL '1' DAY syntax",
			input: "SELECT ts + INTERVAL '1' DAY FROM t",
			want:  "SELECT datetime(ts, '+1 day') FROM t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Translate(tt.input)
			if err != nil {
				t.Fatalf("Translate() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate()\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestTranslateToChar(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "to_char YYYY-MM-DD (strftime fast path)",
			input: "SELECT to_char(ts, 'YYYY-MM-DD') FROM t",
			want:  "SELECT strftime('%Y-%m-%d', ts) FROM t",
		},
		{
			name:  "to_char HH24:MI:SS (strftime fast path)",
			input: "SELECT to_char(ts, 'HH24:MI:SS') FROM t",
			want:  "SELECT strftime('%H:%M:%S', ts) FROM t",
		},
		{
			name:  "to_char with Month (runtime path)",
			input: "SELECT to_char(ts, 'Mon DD, YYYY') FROM t",
			want:  "SELECT pg_to_char(ts, 'Mon DD, YYYY') FROM t",
		},
		{
			name:  "to_char with Day name (runtime path)",
			input: "SELECT to_char(ts, 'Day') FROM t",
			want:  "SELECT pg_to_char(ts, 'Day') FROM t",
		},
		{
			name:  "to_char with AM/PM (runtime path)",
			input: "SELECT to_char(ts, 'HH12:MI AM') FROM t",
			want:  "SELECT pg_to_char(ts, 'HH12:MI AM') FROM t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Translate(tt.input)
			if err != nil {
				t.Fatalf("Translate() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate()\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestTranslateNullsOrdering(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "NULLS FIRST with ASC",
			input: "SELECT * FROM t ORDER BY name ASC NULLS FIRST",
			want:  "SELECT * FROM t ORDER BY (CASE WHEN name IS NULL THEN 0 ELSE 1 END), name ASC",
		},
		{
			name:  "NULLS LAST with ASC",
			input: "SELECT * FROM t ORDER BY name ASC NULLS LAST",
			want:  "SELECT * FROM t ORDER BY (CASE WHEN name IS NULL THEN 1 ELSE 0 END), name ASC",
		},
		{
			name:  "NULLS FIRST with DESC",
			input: "SELECT * FROM t ORDER BY name DESC NULLS FIRST",
			want:  "SELECT * FROM t ORDER BY (CASE WHEN name IS NULL THEN 0 ELSE 1 END), name DESC",
		},
		{
			name:  "NULLS LAST with no explicit direction",
			input: "SELECT * FROM t ORDER BY name NULLS LAST",
			want:  "SELECT * FROM t ORDER BY (CASE WHEN name IS NULL THEN 1 ELSE 0 END), name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Translate(tt.input)
			if err != nil {
				t.Fatalf("Translate() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate()\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestTranslateSimilarTo(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "SIMILAR TO",
			input: "SELECT * FROM t WHERE name SIMILAR TO '%(foo|bar)%'",
			want:  "SELECT * FROM t WHERE pg_similar_match(name, '%(foo|bar)%')",
		},
		{
			name:  "NOT SIMILAR TO",
			input: "SELECT * FROM t WHERE name NOT SIMILAR TO '%test%'",
			want:  "SELECT * FROM t WHERE NOT pg_similar_match(name, '%test%')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Translate(tt.input)
			if err != nil {
				t.Fatalf("Translate() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate()\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestTranslateExplain(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "EXPLAIN SELECT",
			input: "EXPLAIN SELECT * FROM t",
			want:  "EXPLAIN QUERY PLAN SELECT * FROM t",
		},
		{
			name:  "EXPLAIN ANALYZE SELECT",
			input: "EXPLAIN ANALYZE SELECT * FROM t WHERE id = 1",
			want:  "EXPLAIN QUERY PLAN SELECT * FROM t WHERE id = 1",
		},
		{
			name:  "EXPLAIN VERBOSE SELECT",
			input: "EXPLAIN VERBOSE SELECT * FROM t",
			want:  "EXPLAIN QUERY PLAN SELECT * FROM t",
		},
		{
			name:  "EXPLAIN ANALYZE VERBOSE SELECT",
			input: "EXPLAIN ANALYZE VERBOSE SELECT * FROM t",
			want:  "EXPLAIN QUERY PLAN SELECT * FROM t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Translate(tt.input)
			if err != nil {
				t.Fatalf("Translate() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate()\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestDollarQuotedStrings(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple $$",
			input: "SELECT $$hello world$$",
			want:  "SELECT 'hello world'",
		},
		{
			name:  "tagged $fn$",
			input: "SELECT $fn$body text$fn$",
			want:  "SELECT 'body text'",
		},
		{
			name:  "$$ with single quotes inside",
			input: "SELECT $$it's a test$$",
			want:  "SELECT 'it''s a test'",
		},
		{
			name:  "$$ empty string",
			input: "SELECT $$$$",
			want:  "SELECT ''",
		},
		{
			name:  "$$ in INSERT",
			input: "INSERT INTO t (val) VALUES ($$hello$$)",
			want:  "INSERT INTO t (val) VALUES ('hello')",
		},
		{
			name:  "$$ with param still works",
			input: "SELECT $1, $$literal$$",
			want:  "SELECT ?, 'literal'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Translate(tt.input)
			if err != nil {
				t.Fatalf("Translate() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate()\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestTranslatePassthrough(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"simple select", "SELECT 1"},
		{"select with where", "SELECT * FROM t WHERE id = 1"},
		{"insert", "INSERT INTO t (a) VALUES (1)"},
		{"update", "UPDATE t SET a = 1 WHERE id = 2"},
		{"delete", "DELETE FROM t WHERE id = 1"},
		{"create index", "CREATE INDEX idx_t_a ON t (a)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Translate(tt.input)
			if err != nil {
				t.Fatalf("Translate() error: %v", err)
			}
			if got != tt.input {
				t.Errorf("Translate() should pass through\n  got:  %s\n  want: %s", got, tt.input)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	tokens := Tokenize("SELECT 'hello' FROM t WHERE id = $1")
	kinds := make([]TokenKind, len(tokens))
	for i, tok := range tokens {
		kinds[i] = tok.Kind
	}
	// Just verify we get reasonable tokens
	if len(tokens) == 0 {
		t.Fatal("expected tokens, got none")
	}
	if tokens[0].Kind != TokKeyword || tokens[0].Value != "SELECT" {
		t.Errorf("first token: got %v %q, want keyword SELECT", tokens[0].Kind, tokens[0].Value)
	}
}

func TestParseDSN(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{":memory:", ":memory:"},
		{"file:test.db", "file:test.db"},
		{"myapp.db", "myapp.db"},
		{"postgres://user:pass@localhost/myapp", "myapp.db"},
		{"postgresql://user:pass@localhost/myapp", "myapp.db"},
		{"host=localhost dbname=myapp", "myapp.db"},
		{"dbname=test user=postgres", "test.db"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseDSN(tt.input)
			if got != tt.want {
				t.Errorf("parseDSN(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
