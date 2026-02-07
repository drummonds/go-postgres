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
