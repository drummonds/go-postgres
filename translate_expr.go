package pglike

import "strings"

// translateExpressions handles expression-level translations:
// ::cast, ILIKE, TRUE/FALSE literals, E'strings', IS TRUE/FALSE.
func translateExpressions(tokens []Token) []Token {
	tokens = translateCast(tokens)
	tokens = translateILIKE(tokens)
	tokens = translateEscapeStrings(tokens)
	tokens = translateIsTrueFalse(tokens)
	tokens = translateBooleans(tokens)
	return tokens
}

// translateCast converts expr::type to CAST(expr AS mapped_type).
func translateCast(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind == TokOperator && tokens[i].Value == "::" {
			// Find expression to the left (in out)
			exprRef := extractLeftExpr(out)
			// Copy to avoid aliasing with the out slice's backing array
			exprTokens := make([]Token, len(exprRef))
			copy(exprTokens, exprRef)
			// Remove expr tokens from out
			out = out[:len(out)-len(exprTokens)]

			// Read the type name to the right
			typeTokens, end := extractTypeName(tokens, i+1)
			i = end

			// Map the type
			typeName := assembleTypeName(typeTokens)
			mappedType := mapCastType(typeName)

			// Emit CAST(expr AS type)
			out = append(out, Token{Kind: TokKeyword, Value: "CAST", Raw: "CAST"})
			out = append(out, Token{Kind: TokParen, Value: "(", Raw: "("})
			out = append(out, exprTokens...)
			out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
			out = append(out, Token{Kind: TokKeyword, Value: "AS", Raw: "AS"})
			out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
			out = append(out, Token{Kind: TokIdent, Value: mappedType, Raw: mappedType})
			out = append(out, Token{Kind: TokParen, Value: ")", Raw: ")"})
			continue
		}
		out = append(out, tokens[i])
	}
	return out
}

// extractLeftExpr extracts the expression to the left of :: from the output tokens.
// The expression can be: a simple value/ident, a string literal, a number, or a parenthesized group.
func extractLeftExpr(out []Token) []Token {
	if len(out) == 0 {
		return nil
	}
	last := out[len(out)-1]

	// If parenthesized group, find matching open paren
	if last.Kind == TokParen && last.Value == ")" {
		depth := 1
		j := len(out) - 2
		for j >= 0 && depth > 0 {
			if out[j].Kind == TokParen && out[j].Value == ")" {
				depth++
			} else if out[j].Kind == TokParen && out[j].Value == "(" {
				depth--
			}
			if depth > 0 {
				j--
			}
		}
		// Include any function name before the paren
		if j > 0 && (out[j-1].Kind == TokIdent || out[j-1].Kind == TokKeyword) {
			j--
		}
		return out[j:]
	}

	// Simple: ident, string, number, keyword, param
	switch last.Kind {
	case TokIdent, TokString, TokNumber, TokKeyword, TokParam:
		return out[len(out)-1:]
	}

	return out[len(out)-1:]
}

// extractTypeName reads a type name starting at position start.
// Returns the tokens making up the type name and the last index consumed.
func extractTypeName(tokens []Token, start int) ([]Token, int) {
	var result []Token
	i := start
	// Skip whitespace
	for i < len(tokens) && tokens[i].Kind == TokWhitespace {
		i++
	}

	if i >= len(tokens) {
		return result, start
	}

	// Read the type keyword/ident
	if tokens[i].Kind == TokKeyword || tokens[i].Kind == TokIdent {
		result = append(result, tokens[i])
		i++

		// Check for multi-word types: "DOUBLE PRECISION", "TIME ZONE", "CHARACTER VARYING"
		for i < len(tokens) {
			j := i
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}
			if j < len(tokens) && tokens[j].Kind == TokKeyword {
				v := tokens[j].Value
				if v == "PRECISION" || v == "VARYING" || v == "ZONE" || v == "WITH" || v == "WITHOUT" || v == "TIME" {
					result = append(result, tokens[j])
					i = j + 1
					continue
				}
			}
			break
		}

		// Skip optional (n) or (n,m)
		j := i
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
			i = j
		}
	}

	return result, i - 1
}

// assembleTypeName joins type tokens into a single type name string.
func assembleTypeName(tokens []Token) string {
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		parts[i] = t.Value
	}
	return strings.Join(parts, " ")
}

// mapCastType maps a PG type to its SQLite equivalent for CAST.
func mapCastType(pgType string) string {
	upper := strings.ToUpper(pgType)
	switch upper {
	case "INTEGER", "INT", "INT4", "SMALLINT", "INT2", "BIGINT", "INT8":
		return "INTEGER"
	case "REAL", "FLOAT4", "FLOAT8", "DOUBLE PRECISION", "NUMERIC", "DECIMAL":
		return "REAL"
	case "TEXT", "VARCHAR", "CHARACTER VARYING", "CHAR", "CHARACTER", "UUID", "JSON", "JSONB",
		"TIMESTAMP", "TIMESTAMP WITH TIME ZONE", "TIMESTAMP WITHOUT TIME ZONE", "TIMESTAMPTZ",
		"DATE", "TIME", "TIME WITH TIME ZONE", "TIMETZ", "INTERVAL":
		return "TEXT"
	case "BOOLEAN", "BOOL":
		return "INTEGER"
	case "BYTEA":
		return "BLOB"
	}
	return upper
}

// translateILIKE converts ILIKE to LIKE (SQLite LIKE is case-insensitive for ASCII by default).
func translateILIKE(tokens []Token) []Token {
	out := make([]Token, len(tokens))
	copy(out, tokens)
	for i := range out {
		if out[i].Kind == TokKeyword && out[i].Value == "ILIKE" {
			out[i] = Token{Kind: TokKeyword, Value: "LIKE", Raw: "LIKE"}
		}
	}
	return out
}

// translateBooleans converts TRUE -> 1, FALSE -> 0 in non-DDL contexts.
func translateBooleans(tokens []Token) []Token {
	out := make([]Token, len(tokens))
	copy(out, tokens)
	for i := range out {
		if out[i].Kind == TokKeyword {
			switch out[i].Value {
			case "TRUE":
				out[i] = Token{Kind: TokNumber, Value: "1", Raw: "1"}
			case "FALSE":
				out[i] = Token{Kind: TokNumber, Value: "0", Raw: "0"}
			}
		}
	}
	return out
}

// translateEscapeStrings converts E'...' escape strings to regular strings with
// escape sequences resolved.
func translateEscapeStrings(tokens []Token) []Token {
	out := make([]Token, len(tokens))
	copy(out, tokens)
	for i := range out {
		if out[i].Kind == TokString && (strings.HasPrefix(out[i].Raw, "E'") || strings.HasPrefix(out[i].Raw, "e'")) {
			// Extract the content between E' and '
			content := out[i].Raw[2 : len(out[i].Raw)-1]
			// Resolve escape sequences
			resolved := resolveEscapes(content)
			// Re-quote as a standard SQL string
			escaped := strings.ReplaceAll(resolved, "'", "''")
			newRaw := "'" + escaped + "'"
			out[i] = Token{Kind: TokString, Value: newRaw, Raw: newRaw}
		}
	}
	return out
}

// resolveEscapes processes PostgreSQL backslash escape sequences.
func resolveEscapes(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\\' && i+1 < len(runes) {
			i++
			switch runes[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '\\':
				b.WriteByte('\\')
			case '\'':
				b.WriteByte('\'')
			default:
				b.WriteRune('\\')
				b.WriteRune(runes[i])
			}
		} else {
			b.WriteRune(runes[i])
		}
	}
	return b.String()
}

// translateIsTrueFalse converts "IS TRUE" -> "= 1", "IS FALSE" -> "= 0",
// "IS NOT TRUE" -> "!= 1 OR expr IS NULL", "IS NOT FALSE" -> "!= 0 OR expr IS NULL".
func translateIsTrueFalse(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind != TokKeyword || tokens[i].Value != "IS" {
			out = append(out, tokens[i])
			continue
		}

		// Look ahead past whitespace
		j := i + 1
		for j < len(tokens) && tokens[j].Kind == TokWhitespace {
			j++
		}

		// IS NOT TRUE / IS NOT FALSE
		if j < len(tokens) && tokens[j].Kind == TokKeyword && tokens[j].Value == "NOT" {
			k := j + 1
			for k < len(tokens) && tokens[k].Kind == TokWhitespace {
				k++
			}
			if k < len(tokens) && tokens[k].Kind == TokKeyword {
				switch tokens[k].Value {
				case "TRUE":
					// IS NOT TRUE -> != 1
					out = append(out,
						Token{Kind: TokOperator, Value: "!=", Raw: "!="},
						Token{Kind: TokWhitespace, Value: " ", Raw: " "},
						Token{Kind: TokNumber, Value: "1", Raw: "1"},
					)
					i = k
					continue
				case "FALSE":
					// IS NOT FALSE -> != 0
					out = append(out,
						Token{Kind: TokOperator, Value: "!=", Raw: "!="},
						Token{Kind: TokWhitespace, Value: " ", Raw: " "},
						Token{Kind: TokNumber, Value: "0", Raw: "0"},
					)
					i = k
					continue
				}
			}
		}

		// IS TRUE / IS FALSE
		if j < len(tokens) && tokens[j].Kind == TokKeyword {
			switch tokens[j].Value {
			case "TRUE":
				out = append(out,
					Token{Kind: TokOperator, Value: "=", Raw: "="},
					Token{Kind: TokWhitespace, Value: " ", Raw: " "},
					Token{Kind: TokNumber, Value: "1", Raw: "1"},
				)
				i = j
				continue
			case "FALSE":
				out = append(out,
					Token{Kind: TokOperator, Value: "=", Raw: "="},
					Token{Kind: TokWhitespace, Value: " ", Raw: " "},
					Token{Kind: TokNumber, Value: "0", Raw: "0"},
				)
				i = j
				continue
			}
		}

		// IS NULL / IS NOT NULL — pass through
		out = append(out, tokens[i])
	}
	return out
}
