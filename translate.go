package pglike

import (
	"strings"
	"unicode"
)

// TokenKind classifies a SQL token.
type TokenKind int

const (
	TokKeyword    TokenKind = iota // SQL keyword (uppercased for comparison)
	TokIdent                       // identifier (table/column name)
	TokString                      // string literal 'foo'
	TokNumber                      // numeric literal
	TokOperator                    // operator (::, =, <, >, etc.)
	TokParam                       // $1, $2 placeholder
	TokParen                       // ( or )
	TokComma                       // ,
	TokSemicolon                   // ;
	TokWhitespace                  // spaces, tabs, newlines
	TokComment                     // -- or /* */
	TokDot                         // .
)

// Token represents a single token from SQL input.
type Token struct {
	Kind  TokenKind
	Value string // normalized value (uppercased for keywords)
	Raw   string // original text
}

// sqlKeywords is the set of SQL keywords we recognize.
var sqlKeywords = map[string]bool{
	"SELECT": true, "FROM": true, "WHERE": true, "INSERT": true, "INTO": true,
	"UPDATE": true, "DELETE": true, "CREATE": true, "TABLE": true, "DROP": true,
	"ALTER": true, "ADD": true, "COLUMN": true, "INDEX": true, "IF": true,
	"NOT": true, "EXISTS": true, "NULL": true, "DEFAULT": true, "PRIMARY": true,
	"KEY": true, "UNIQUE": true, "CHECK": true, "FOREIGN": true, "REFERENCES": true,
	"ON": true, "SET": true, "VALUES": true, "AND": true, "OR": true,
	"IN": true, "IS": true, "AS": true, "JOIN": true, "LEFT": true,
	"RIGHT": true, "INNER": true, "OUTER": true, "CROSS": true, "FULL": true,
	"ORDER": true, "BY": true, "ASC": true, "DESC": true, "GROUP": true,
	"HAVING": true, "LIMIT": true, "OFFSET": true, "UNION": true, "ALL": true,
	"DISTINCT": true, "CASE": true, "WHEN": true, "THEN": true, "ELSE": true,
	"END": true, "BETWEEN": true, "LIKE": true, "ILIKE": true, "SIMILAR": true,
	"TO": true, "CAST": true, "TRUE": true, "FALSE": true, "BEGIN": true,
	"COMMIT": true, "ROLLBACK": true, "RETURNING": true, "WITH": true,
	"RECURSIVE": true, "EXCEPT": true, "INTERSECT": true, "CONSTRAINT": true,
	"CASCADE": true, "RESTRICT": true, "AUTOINCREMENT": true,

	// Type keywords
	"SERIAL": true, "BIGSERIAL": true, "SMALLSERIAL": true,
	"BOOLEAN": true, "BOOL": true,
	"VARCHAR": true, "CHARACTER": true, "VARYING": true, "CHAR": true,
	"TEXT": true, "INTEGER": true, "INT": true, "INT2": true, "INT4": true,
	"INT8": true, "SMALLINT": true, "BIGINT": true,
	"REAL": true, "FLOAT4": true, "FLOAT8": true, "DOUBLE": true, "PRECISION": true,
	"NUMERIC": true, "DECIMAL": true,
	"TIMESTAMP": true, "TIMESTAMPTZ": true, "DATE": true, "TIME": true, "TIMETZ": true,
	"UUID": true, "BYTEA": true, "JSON": true, "JSONB": true, "BLOB": true,
	"ZONE": true,

	// Function-like keywords
	"NOW": true, "CURRENT_DATE": true, "CURRENT_TIME": true, "CURRENT_TIMESTAMP": true,
	"EXTRACT": true, "COALESCE": true, "NULLIF": true,

	// Additional
	"REPLACE": true, "CONFLICT": true, "DO": true, "NOTHING": true,
	"OVER": true, "PARTITION": true, "WINDOW": true, "ROW": true, "ROWS": true,
	"RANGE": true, "PRECEDING": true, "FOLLOWING": true, "UNBOUNDED": true,
	"CURRENT": true, "EXCLUDE": true, "TIES": true, "OTHERS": true,
	"NO": true, "ACTION": true, "DEFERRABLE": true, "INITIALLY": true,
	"DEFERRED": true, "IMMEDIATE": true, "ONLY": true, "TEMPORARY": true,
	"TEMP": true, "UNLOGGED": true, "MATERIALIZED": true, "VIEW": true,
	"USING": true, "NATURAL": true, "LATERAL": true, "FETCH": true,
	"FIRST": true, "LAST": true, "NEXT": true, "PRIOR": true,
	"ABSOLUTE": true, "RELATIVE": true, "FORWARD": true, "BACKWARD": true,
	"SOME": true, "ANY": true, "EVERY": true, "ARRAY": true,
	"INTERVAL": true, "WITHOUT": true,

	// Phase 2 keywords
	"NULLS": true, "SEQUENCE": true, "INCREMENT": true, "START": true,
	"MINVALUE": true, "MAXVALUE": true, "CYCLE": true, "OWNED": true,
	"EXPLAIN": true, "ANALYZE": true, "VERBOSE": true, "PLAN": true,
	"QUERY": true,
}

// Tokenize splits a SQL string into tokens.
func Tokenize(sql string) []Token {
	var tokens []Token
	i := 0
	runes := []rune(sql)
	n := len(runes)

	for i < n {
		ch := runes[i]

		// Whitespace
		if unicode.IsSpace(ch) {
			start := i
			for i < n && unicode.IsSpace(runes[i]) {
				i++
			}
			raw := string(runes[start:i])
			tokens = append(tokens, Token{Kind: TokWhitespace, Value: raw, Raw: raw})
			continue
		}

		// Line comment --
		if ch == '-' && i+1 < n && runes[i+1] == '-' {
			start := i
			for i < n && runes[i] != '\n' {
				i++
			}
			raw := string(runes[start:i])
			tokens = append(tokens, Token{Kind: TokComment, Value: raw, Raw: raw})
			continue
		}

		// Block comment /* */
		if ch == '/' && i+1 < n && runes[i+1] == '*' {
			start := i
			i += 2
			for i+1 < n && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			if i+1 < n {
				i += 2
			}
			raw := string(runes[start:i])
			tokens = append(tokens, Token{Kind: TokComment, Value: raw, Raw: raw})
			continue
		}

		// E'escape string'
		if (ch == 'E' || ch == 'e') && i+1 < n && runes[i+1] == '\'' {
			start := i
			i += 2 // skip E'
			for i < n {
				if runes[i] == '\\' && i+1 < n {
					i += 2 // skip escaped char
				} else if runes[i] == '\'' {
					i++
					break
				} else {
					i++
				}
			}
			raw := string(runes[start:i])
			tokens = append(tokens, Token{Kind: TokString, Value: raw, Raw: raw})
			continue
		}

		// String literal 'foo'
		if ch == '\'' {
			start := i
			i++
			for i < n {
				if runes[i] == '\'' && i+1 < n && runes[i+1] == '\'' {
					i += 2 // escaped quote ''
				} else if runes[i] == '\'' {
					i++
					break
				} else {
					i++
				}
			}
			raw := string(runes[start:i])
			tokens = append(tokens, Token{Kind: TokString, Value: raw, Raw: raw})
			continue
		}

		// Quoted identifier "foo"
		if ch == '"' {
			start := i
			i++
			for i < n && runes[i] != '"' {
				i++
			}
			if i < n {
				i++
			}
			raw := string(runes[start:i])
			tokens = append(tokens, Token{Kind: TokIdent, Value: raw, Raw: raw})
			continue
		}

		// Dollar-quoted string $$...$$ or $tag$...$tag$
		if ch == '$' {
			if tag, end, ok := tryDollarQuote(runes, i, n); ok {
				// Extract content between tags
				content := string(runes[i+len(tag) : end-len(tag)])
				// Convert to standard SQL string
				escaped := strings.ReplaceAll(content, "'", "''")
				newRaw := "'" + escaped + "'"
				tokens = append(tokens, Token{Kind: TokString, Value: newRaw, Raw: newRaw})
				i = end
				continue
			}
			// Parameter $1, $2, ...
			if i+1 < n && unicode.IsDigit(runes[i+1]) {
				start := i
				i++
				for i < n && unicode.IsDigit(runes[i]) {
					i++
				}
				raw := string(runes[start:i])
				tokens = append(tokens, Token{Kind: TokParam, Value: raw, Raw: raw})
				continue
			}
			// Lone $ — emit as operator
			tokens = append(tokens, Token{Kind: TokOperator, Value: "$", Raw: "$"})
			i++
			continue
		}

		// Number
		if unicode.IsDigit(ch) || (ch == '.' && i+1 < n && unicode.IsDigit(runes[i+1])) {
			start := i
			for i < n && (unicode.IsDigit(runes[i]) || runes[i] == '.') {
				i++
			}
			// Handle scientific notation
			if i < n && (runes[i] == 'e' || runes[i] == 'E') {
				i++
				if i < n && (runes[i] == '+' || runes[i] == '-') {
					i++
				}
				for i < n && unicode.IsDigit(runes[i]) {
					i++
				}
			}
			raw := string(runes[start:i])
			tokens = append(tokens, Token{Kind: TokNumber, Value: raw, Raw: raw})
			continue
		}

		// Operator ::
		if ch == ':' && i+1 < n && runes[i+1] == ':' {
			tokens = append(tokens, Token{Kind: TokOperator, Value: "::", Raw: "::"})
			i += 2
			continue
		}

		// Regex operators !~* !~ ~*
		if ch == '!' && i+1 < n && runes[i+1] == '~' {
			if i+2 < n && runes[i+2] == '*' {
				tokens = append(tokens, Token{Kind: TokOperator, Value: "!~*", Raw: "!~*"})
				i += 3
			} else {
				tokens = append(tokens, Token{Kind: TokOperator, Value: "!~", Raw: "!~"})
				i += 2
			}
			continue
		}
		if ch == '~' && i+1 < n && runes[i+1] == '*' {
			tokens = append(tokens, Token{Kind: TokOperator, Value: "~*", Raw: "~*"})
			i += 2
			continue
		}

		// Multi-char operators
		if ch == '<' || ch == '>' || ch == '!' || ch == '=' {
			start := i
			i++
			if i < n && (runes[i] == '=' || runes[i] == '>') {
				i++
			}
			raw := string(runes[start:i])
			tokens = append(tokens, Token{Kind: TokOperator, Value: raw, Raw: raw})
			continue
		}

		// JSON operators -> ->>
		if ch == '-' && i+1 < n && runes[i+1] == '>' {
			if i+2 < n && runes[i+2] == '>' {
				tokens = append(tokens, Token{Kind: TokOperator, Value: "->>", Raw: "->>"})
				i += 3
			} else {
				tokens = append(tokens, Token{Kind: TokOperator, Value: "->", Raw: "->"})
				i += 2
			}
			continue
		}

		// || concatenation
		if ch == '|' && i+1 < n && runes[i+1] == '|' {
			tokens = append(tokens, Token{Kind: TokOperator, Value: "||", Raw: "||"})
			i += 2
			continue
		}

		// Single-char operators
		if ch == '+' || ch == '-' || ch == '*' || ch == '/' || ch == '%' || ch == '|' || ch == '&' || ch == '~' || ch == ':' {
			raw := string(ch)
			tokens = append(tokens, Token{Kind: TokOperator, Value: raw, Raw: raw})
			i++
			continue
		}

		// Parentheses
		if ch == '(' || ch == ')' {
			raw := string(ch)
			tokens = append(tokens, Token{Kind: TokParen, Value: raw, Raw: raw})
			i++
			continue
		}

		// Comma
		if ch == ',' {
			tokens = append(tokens, Token{Kind: TokComma, Value: ",", Raw: ","})
			i++
			continue
		}

		// Semicolon
		if ch == ';' {
			tokens = append(tokens, Token{Kind: TokSemicolon, Value: ";", Raw: ";"})
			i++
			continue
		}

		// Dot
		if ch == '.' {
			tokens = append(tokens, Token{Kind: TokDot, Value: ".", Raw: "."})
			i++
			continue
		}

		// Keyword or identifier
		if ch == '_' || unicode.IsLetter(ch) {
			start := i
			for i < n && (runes[i] == '_' || unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i])) {
				i++
			}
			raw := string(runes[start:i])
			upper := strings.ToUpper(raw)
			if sqlKeywords[upper] {
				tokens = append(tokens, Token{Kind: TokKeyword, Value: upper, Raw: raw})
			} else {
				tokens = append(tokens, Token{Kind: TokIdent, Value: raw, Raw: raw})
			}
			continue
		}

		// Unknown character — emit as operator
		raw := string(ch)
		tokens = append(tokens, Token{Kind: TokOperator, Value: raw, Raw: raw})
		i++
	}

	return tokens
}

// Reassemble converts tokens back into a SQL string.
func Reassemble(tokens []Token) string {
	var b strings.Builder
	for _, t := range tokens {
		b.WriteString(t.Raw)
	}
	return b.String()
}

// Translate converts PostgreSQL SQL to SQLite-compatible SQL.
func Translate(sql string) (string, error) {
	tokens := Tokenize(sql)
	tokens = translateDDL(tokens)
	tokens = translateExpressions(tokens)
	tokens = translateFunctions(tokens)
	tokens = translateNullsOrdering(tokens)
	tokens = translateParams(tokens)
	return Reassemble(tokens), nil
}

// tryDollarQuote checks if runes[i:] starts a dollar-quoted string ($$...$$ or $tag$...$tag$).
// Returns the opening tag (including $ delimiters), the end position, and whether it matched.
func tryDollarQuote(runes []rune, i, n int) (tag []rune, end int, ok bool) {
	// Must start with $
	if i >= n || runes[i] != '$' {
		return nil, 0, false
	}

	// Find the end of the opening tag: $$ or $identifier$
	j := i + 1
	if j >= n {
		return nil, 0, false
	}

	if runes[j] == '$' {
		// $$ tag
		tag = runes[i : j+1] // "$$"
	} else if runes[j] == '_' || unicode.IsLetter(runes[j]) {
		// $identifier$ tag
		k := j
		for k < n && (runes[k] == '_' || unicode.IsLetter(runes[k]) || unicode.IsDigit(runes[k])) {
			k++
		}
		if k >= n || runes[k] != '$' {
			return nil, 0, false
		}
		tag = runes[i : k+1] // "$tag$"
	} else {
		return nil, 0, false
	}

	// Search for the closing tag
	tagLen := len(tag)
	searchStart := i + tagLen
	for p := searchStart; p+tagLen <= n; p++ {
		match := true
		for q := 0; q < tagLen; q++ {
			if runes[p+q] != tag[q] {
				match = false
				break
			}
		}
		if match {
			return tag, p + tagLen, true
		}
	}

	return nil, 0, false
}

// translateParams converts $1, $2, ... to ? placeholders.
func translateParams(tokens []Token) []Token {
	out := make([]Token, len(tokens))
	copy(out, tokens)
	for i := range out {
		if out[i].Kind == TokParam {
			out[i] = Token{Kind: TokOperator, Value: "?", Raw: "?"}
		}
	}
	return out
}
