package pglike

import "strings"

// translateDDL handles DDL-specific translations: type mappings, SERIAL, etc.
func translateDDL(tokens []Token) []Token {
	tokens = translateTypes(tokens)
	tokens = translateSerial(tokens)
	tokens = translateDefaultNow(tokens)
	tokens = translateAlterTableAddColumn(tokens)
	return tokens
}

// translateAlterTableAddColumn strips IF NOT EXISTS from ALTER TABLE ADD COLUMN
// since SQLite does not support that syntax. The driver layer handles suppressing
// duplicate column errors when IF NOT EXISTS was present in the original query.
func translateAlterTableAddColumn(tokens []Token) []Token {
	// Look for pattern: ALTER TABLE <name> ADD [COLUMN] IF NOT EXISTS
	var out []Token
	for i := 0; i < len(tokens); i++ {
		// Match IF NOT EXISTS after ADD or ADD COLUMN
		if tokens[i].Kind == TokKeyword && tokens[i].Value == "IF" {
			// Check: IF NOT EXISTS
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}
			if j < len(tokens) && tokens[j].Kind == TokKeyword && tokens[j].Value == "NOT" {
				k := j + 1
				for k < len(tokens) && tokens[k].Kind == TokWhitespace {
					k++
				}
				if k < len(tokens) && tokens[k].Kind == TokKeyword && tokens[k].Value == "EXISTS" {
					// Check if this follows ADD or ADD COLUMN by looking backwards
					if isAfterAddColumn(out) {
						// Skip IF NOT EXISTS and any trailing whitespace
						i = k
						// Also skip one whitespace after EXISTS if present
						if i+1 < len(tokens) && tokens[i+1].Kind == TokWhitespace {
							i++
						}
						continue
					}
				}
			}
		}
		out = append(out, tokens[i])
	}
	return out
}

// isAfterAddColumn checks if the last non-whitespace tokens in out are ADD [COLUMN].
func isAfterAddColumn(tokens []Token) bool {
	pos := len(tokens)
	// Skip trailing whitespace
	for pos > 0 && tokens[pos-1].Kind == TokWhitespace {
		pos--
	}
	if pos == 0 {
		return false
	}
	// Check for COLUMN (optional)
	if tokens[pos-1].Kind == TokKeyword && tokens[pos-1].Value == "COLUMN" {
		pos--
		for pos > 0 && tokens[pos-1].Kind == TokWhitespace {
			pos--
		}
	}
	// Must end with ADD
	return pos > 0 && tokens[pos-1].Kind == TokKeyword && tokens[pos-1].Value == "ADD"
}

// translateSerial replaces SERIAL/BIGSERIAL/SMALLSERIAL with INTEGER PRIMARY KEY AUTOINCREMENT.
// It detects "colname SERIAL ... [PRIMARY KEY]" and normalizes to
// "colname INTEGER PRIMARY KEY AUTOINCREMENT ...", stripping any PRIMARY KEY
// (and preceding CONSTRAINT name) that appears later in the column definition.
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

			// Collect remaining tokens in this column definition (up to , or ))
			// and strip out [CONSTRAINT name] PRIMARY KEY to avoid duplication.
			var rest []Token
			j := i + 1
			for j < len(tokens) && tokens[j].Kind != TokComma && !(tokens[j].Kind == TokParen && tokens[j].Value == ")") {
				rest = append(rest, tokens[j])
				j++
			}
			rest = stripPrimaryKey(rest)
			// Remove trailing whitespace from rest so we don't get extra spaces before , or )
			for len(rest) > 0 && rest[len(rest)-1].Kind == TokWhitespace {
				rest = rest[:len(rest)-1]
			}
			out = append(out, rest...)
			i = j - 1 // loop will i++ to j
			continue
		}
		out = append(out, t)
	}
	return out
}

// stripPrimaryKey removes PRIMARY KEY (and any preceding CONSTRAINT name) from a token slice.
func stripPrimaryKey(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		// Check for CONSTRAINT <name> PRIMARY KEY
		if tokens[i].Kind == TokKeyword && tokens[i].Value == "CONSTRAINT" {
			// Look ahead: whitespace, name, whitespace, PRIMARY, whitespace, KEY
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}
			if j < len(tokens) && (tokens[j].Kind == TokIdent || tokens[j].Kind == TokKeyword) {
				k := j + 1
				for k < len(tokens) && tokens[k].Kind == TokWhitespace {
					k++
				}
				if k < len(tokens) && tokens[k].Kind == TokKeyword && tokens[k].Value == "PRIMARY" {
					l := k + 1
					for l < len(tokens) && tokens[l].Kind == TokWhitespace {
						l++
					}
					if l < len(tokens) && tokens[l].Kind == TokKeyword && tokens[l].Value == "KEY" {
						// Skip preceding whitespace, CONSTRAINT name PRIMARY KEY
						i = l
						continue
					}
				}
			}
			out = append(out, tokens[i])
			continue
		}
		// Check for bare PRIMARY KEY
		if tokens[i].Kind == TokKeyword && tokens[i].Value == "PRIMARY" {
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}
			if j < len(tokens) && tokens[j].Kind == TokKeyword && tokens[j].Value == "KEY" {
				// Skip preceding whitespace and PRIMARY KEY
				i = j
				continue
			}
		}
		out = append(out, tokens[i])
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

// translateDefaultNow converts DEFAULT NOW() and DEFAULT CURRENT_TIMESTAMP/CURRENT_DATE/CURRENT_TIME
// to DEFAULT (datetime('now')), DEFAULT (date('now')), or DEFAULT (time('now')).
// SQLite requires function calls in DEFAULT clauses to be wrapped in parentheses.
func translateDefaultNow(tokens []Token) []Token {
	// Map of CURRENT_* keywords to their SQLite function equivalents.
	currentFuncMap := map[string]string{
		"CURRENT_TIMESTAMP": "datetime",
		"CURRENT_DATE":      "date",
		"CURRENT_TIME":      "time",
	}

	var out []Token
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]

		if t.Kind == TokKeyword && t.Value == "DEFAULT" {
			out = append(out, t)
			// Look ahead past whitespace
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}

			// Check for NOW()
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

			// Check for CURRENT_TIMESTAMP, CURRENT_DATE, CURRENT_TIME
			if j < len(tokens) && tokens[j].Kind == TokKeyword {
				if funcName, ok := currentFuncMap[tokens[j].Value]; ok {
					out = append(out,
						Token{Kind: TokWhitespace, Value: " ", Raw: " "},
						Token{Kind: TokParen, Value: "(", Raw: "("},
						Token{Kind: TokIdent, Value: funcName, Raw: funcName},
						Token{Kind: TokParen, Value: "(", Raw: "("},
						Token{Kind: TokString, Value: "'now'", Raw: "'now'"},
						Token{Kind: TokParen, Value: ")", Raw: ")"},
						Token{Kind: TokParen, Value: ")", Raw: ")"},
					)
					i = j
					continue
				}
			}

			// Not a recognized datetime default, just pass through DEFAULT
			continue
		}
		out = append(out, t)
	}
	return out
}
