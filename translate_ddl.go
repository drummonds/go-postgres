package pglike

import "strings"

// translateDDL handles DDL-specific translations: type mappings, SERIAL, etc.
func translateDDL(tokens []Token) []Token {
	tokens = translateTypes(tokens)
	tokens = translateSerial(tokens)
	tokens = translateDefaultNow(tokens)
	return tokens
}

// translateSerial replaces SERIAL/BIGSERIAL/SMALLSERIAL with INTEGER PRIMARY KEY AUTOINCREMENT.
// It detects "colname SERIAL [PRIMARY KEY]" and normalizes to
// "colname INTEGER PRIMARY KEY AUTOINCREMENT".
func translateSerial(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t.Kind == TokKeyword && (t.Value == "SERIAL" || t.Value == "BIGSERIAL" || t.Value == "SMALLSERIAL") {
			// Replace with INTEGER PRIMARY KEY AUTOINCREMENT
			out = append(out, Token{Kind: TokKeyword, Value: "INTEGER", Raw: "INTEGER"})
			out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
			out = append(out, Token{Kind: TokKeyword, Value: "PRIMARY", Raw: "PRIMARY"})
			out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
			out = append(out, Token{Kind: TokKeyword, Value: "KEY", Raw: "KEY"})
			out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
			out = append(out, Token{Kind: TokKeyword, Value: "AUTOINCREMENT", Raw: "AUTOINCREMENT"})

			// If PRIMARY KEY follows, skip past it to avoid duplication
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}
			if j < len(tokens) && tokens[j].Kind == TokKeyword && tokens[j].Value == "PRIMARY" {
				k := j + 1
				for k < len(tokens) && tokens[k].Kind == TokWhitespace {
					k++
				}
				if k < len(tokens) && tokens[k].Kind == TokKeyword && tokens[k].Value == "KEY" {
					i = k
				}
			}
			continue
		}
		out = append(out, t)
	}
	return out
}

// pgTypeToSQLite maps PG type names to SQLite type names.
var pgTypeToSQLite = map[string]string{
	"BOOLEAN":   "INTEGER",
	"BOOL":      "INTEGER",
	"VARCHAR":   "TEXT",
	"CHARACTER": "TEXT", // starts CHARACTER VARYING
	"CHAR":      "TEXT",
	"TIMESTAMP": "TEXT",
	"TIMESTAMPTZ": "TEXT",
	"DATE":      "TEXT",
	"TIME":      "TEXT",
	"TIMETZ":    "TEXT",
	"UUID":      "TEXT",
	"BYTEA":     "BLOB",
	"JSON":      "TEXT",
	"JSONB":     "TEXT",
	"SMALLINT":  "INTEGER",
	"INT2":      "INTEGER",
	"INT4":      "INTEGER",
	"INT8":      "INTEGER",
	"BIGINT":    "INTEGER",
	"FLOAT4":    "REAL",
	"FLOAT8":    "REAL",
}

// MapType maps a PostgreSQL type name to its SQLite equivalent.
func MapType(pgType string) string {
	upper := strings.ToUpper(pgType)
	if mapped, ok := pgTypeToSQLite[upper]; ok {
		return mapped
	}
	return pgType
}

// translateTypes handles PG type names in DDL, replacing them with SQLite equivalents.
// Handles multi-word types like "DOUBLE PRECISION", "CHARACTER VARYING", "TIMESTAMP WITH TIME ZONE".
func translateTypes(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]

		if t.Kind != TokKeyword {
			out = append(out, t)
			continue
		}

		switch t.Value {
		case "DOUBLE":
			// DOUBLE PRECISION -> REAL
			if j, ok := peekKeyword(tokens, i+1, "PRECISION"); ok {
				out = append(out, Token{Kind: TokKeyword, Value: "REAL", Raw: "REAL"})
				i = j
				continue
			}
			out = append(out, t)

		case "CHARACTER":
			// CHARACTER VARYING(n) -> TEXT or CHARACTER(n) -> TEXT
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}
			if j < len(tokens) && tokens[j].Kind == TokKeyword && tokens[j].Value == "VARYING" {
				// CHARACTER VARYING -> TEXT, skip (n)
				out = append(out, Token{Kind: TokKeyword, Value: "TEXT", Raw: "TEXT"})
				i = j
				i = skipParenGroup(tokens, i+1)
				continue
			}
			// CHARACTER(n) -> TEXT
			out = append(out, Token{Kind: TokKeyword, Value: "TEXT", Raw: "TEXT"})
			i = skipParenGroup(tokens, i+1)
			continue

		case "VARCHAR", "CHAR":
			// VARCHAR(n) -> TEXT, skip (n)
			out = append(out, Token{Kind: TokKeyword, Value: "TEXT", Raw: "TEXT"})
			i = skipParenGroup(tokens, i+1)
			continue

		case "NUMERIC", "DECIMAL":
			// NUMERIC(p,s) -> REAL
			out = append(out, Token{Kind: TokKeyword, Value: "REAL", Raw: "REAL"})
			i = skipParenGroup(tokens, i+1)
			continue

		case "TIMESTAMP":
			// TIMESTAMP WITH TIME ZONE -> TEXT, or TIMESTAMP WITHOUT TIME ZONE -> TEXT
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}
			if j < len(tokens) && tokens[j].Kind == TokKeyword && (tokens[j].Value == "WITH" || tokens[j].Value == "WITHOUT") {
				k := j + 1
				for k < len(tokens) && tokens[k].Kind == TokWhitespace {
					k++
				}
				if k < len(tokens) && tokens[k].Kind == TokKeyword && tokens[k].Value == "TIME" {
					l := k + 1
					for l < len(tokens) && tokens[l].Kind == TokWhitespace {
						l++
					}
					if l < len(tokens) && tokens[l].Kind == TokKeyword && tokens[l].Value == "ZONE" {
						out = append(out, Token{Kind: TokKeyword, Value: "TEXT", Raw: "TEXT"})
						i = l
						continue
					}
				}
			}
			out = append(out, Token{Kind: TokKeyword, Value: "TEXT", Raw: "TEXT"})
			continue

		case "INTERVAL":
			// INTERVAL -> TEXT (column type only; arithmetic INTERVAL handled by translateInterval)
			out = append(out, Token{Kind: TokKeyword, Value: "TEXT", Raw: "TEXT"})
			continue

		case "TIME":
			// TIME WITH TIME ZONE -> TEXT
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}
			if j < len(tokens) && tokens[j].Kind == TokKeyword && (tokens[j].Value == "WITH" || tokens[j].Value == "WITHOUT") {
				k := j + 1
				for k < len(tokens) && tokens[k].Kind == TokWhitespace {
					k++
				}
				if k < len(tokens) && tokens[k].Kind == TokKeyword && tokens[k].Value == "TIME" {
					l := k + 1
					for l < len(tokens) && tokens[l].Kind == TokWhitespace {
						l++
					}
					if l < len(tokens) && tokens[l].Kind == TokKeyword && tokens[l].Value == "ZONE" {
						out = append(out, Token{Kind: TokKeyword, Value: "TEXT", Raw: "TEXT"})
						i = l
						continue
					}
				}
			}
			out = append(out, Token{Kind: TokKeyword, Value: "TEXT", Raw: "TEXT"})
			continue

		default:
			if mapped, ok := pgTypeToSQLite[t.Value]; ok {
				out = append(out, Token{Kind: TokKeyword, Value: mapped, Raw: mapped})
			} else {
				out = append(out, t)
			}
		}
	}
	return out
}

// peekKeyword looks past whitespace for an expected keyword, returning the index and true if found.
func peekKeyword(tokens []Token, start int, keyword string) (int, bool) {
	j := start
	for j < len(tokens) && tokens[j].Kind == TokWhitespace {
		j++
	}
	if j < len(tokens) && tokens[j].Kind == TokKeyword && tokens[j].Value == keyword {
		return j, true
	}
	return start, false
}

// skipParenGroup skips past whitespace and a parenthesized group like (100) or (10,2).
// Returns the index of the last token consumed (the closing paren), or start-1 if no paren found.
func skipParenGroup(tokens []Token, start int) int {
	j := start
	for j < len(tokens) && tokens[j].Kind == TokWhitespace {
		j++
	}
	if j < len(tokens) && tokens[j].Kind == TokParen && tokens[j].Value == "(" {
		depth := 1
		j++
		for j < len(tokens) && depth > 0 {
			if tokens[j].Kind == TokParen && tokens[j].Value == "(" {
				depth++
			} else if tokens[j].Kind == TokParen && tokens[j].Value == ")" {
				depth--
			}
			j++
		}
		return j - 1 // index of closing paren
	}
	return start - 1 // no paren, don't skip anything
}

// translateDefaultNow converts DEFAULT NOW() to DEFAULT (datetime('now')).
func translateDefaultNow(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]

		if t.Kind == TokKeyword && t.Value == "DEFAULT" {
			out = append(out, t)
			// Look ahead for NOW()
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}
			if j < len(tokens) && tokens[j].Kind == TokKeyword && tokens[j].Value == "NOW" {
				k := j + 1
				for k < len(tokens) && tokens[k].Kind == TokWhitespace {
					k++
				}
				if k < len(tokens) && tokens[k].Kind == TokParen && tokens[k].Value == "(" {
					l := k + 1
					for l < len(tokens) && tokens[l].Kind == TokWhitespace {
						l++
					}
					if l < len(tokens) && tokens[l].Kind == TokParen && tokens[l].Value == ")" {
						// Replace DEFAULT NOW() with DEFAULT (datetime('now'))
						// Emit one space then the replacement
						out = append(out,
							Token{Kind: TokWhitespace, Value: " ", Raw: " "},
							Token{Kind: TokParen, Value: "(", Raw: "("},
							Token{Kind: TokIdent, Value: "datetime", Raw: "datetime"},
							Token{Kind: TokParen, Value: "(", Raw: "("},
							Token{Kind: TokString, Value: "'now'", Raw: "'now'"},
							Token{Kind: TokParen, Value: ")", Raw: ")"},
							Token{Kind: TokParen, Value: ")", Raw: ")"},
						)
						i = l
						continue
					}
				}
			}
			// Not NOW(), just pass through DEFAULT (don't duplicate whitespace)
			continue
		}
		out = append(out, t)
	}
	return out
}
