package pglike

import "testing"

func BenchmarkTranslate_SimpleSelect(b *testing.B) {
	sql := "SELECT id, name, balance FROM accounts WHERE name ILIKE $1 AND active IS TRUE ORDER BY created_at DESC LIMIT 100"
	b.ReportAllocs()
	for b.Loop() {
		Translate(sql)
	}
}

func BenchmarkTranslate_ComplexDDL(b *testing.B) {
	sql := `CREATE TABLE IF NOT EXISTS users (
		id UUID DEFAULT (gen_random_uuid()) PRIMARY KEY,
		email VARCHAR(255) NOT NULL UNIQUE,
		name VARCHAR(100),
		active BOOLEAN DEFAULT TRUE,
		balance BIGINT NOT NULL DEFAULT 0,
		metadata JSONB,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	)`
	b.ReportAllocs()
	for b.Loop() {
		Translate(sql)
	}
}

func BenchmarkTranslate_InsertReturning(b *testing.B) {
	sql := "INSERT INTO accounts (name, email, balance) VALUES ($1, $2, $3) RETURNING id, created_at"
	b.ReportAllocs()
	for b.Loop() {
		Translate(sql)
	}
}

func BenchmarkTokenize(b *testing.B) {
	sql := "SELECT id, name, balance FROM accounts WHERE name ILIKE $1 AND active IS TRUE ORDER BY created_at DESC LIMIT 100"
	b.ReportAllocs()
	for b.Loop() {
		Tokenize(sql)
	}
}

func BenchmarkTranslateTokens(b *testing.B) {
	sql := "SELECT id, name, balance FROM accounts WHERE name ILIKE $1 AND active IS TRUE ORDER BY created_at DESC LIMIT 100"
	tokens := Tokenize(sql)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		translateTokens(tokens)
	}
}

func BenchmarkTranslateCached_SimpleSelect(b *testing.B) {
	sql := "SELECT id, name, balance FROM accounts WHERE name ILIKE $1 AND active IS TRUE ORDER BY created_at DESC LIMIT 100"
	// Prime cache
	TranslateCached(sql)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		TranslateCached(sql)
	}
}
