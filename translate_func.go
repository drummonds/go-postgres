package pglike

import "strings"

// translateFunctions handles function-level translations:
// NOW(), date_trunc(), EXTRACT(), string functions, etc.
func translateFunctions(tokens []Token) []Token {
	tokens = translateNow(tokens)
	tokens = translateCurrentDatetime(tokens)
	tokens = translateDateTrunc(tokens)
	tokens = translateExtract(tokens)
	tokens = translateStringFuncs(tokens)
	tokens = translateAggFuncs(tokens)
	return tokens
}

// translateNow converts NOW() -> datetime('now') (not in DEFAULT context, handled by DDL).
func translateNow(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind == TokKeyword && tokens[i].Value == "NOW" {
			// Look ahead for ()
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}
			if j < len(tokens) && tokens[j].Kind == TokParen && tokens[j].Value == "(" {
				k := j + 1
				for k < len(tokens) && tokens[k].Kind == TokWhitespace {
					k++
				}
				if k < len(tokens) && tokens[k].Kind == TokParen && tokens[k].Value == ")" {
					out = append(out,
						Token{Kind: TokIdent, Value: "datetime", Raw: "datetime"},
						Token{Kind: TokParen, Value: "(", Raw: "("},
						Token{Kind: TokString, Value: "'now'", Raw: "'now'"},
						Token{Kind: TokParen, Value: ")", Raw: ")"},
					)
					i = k
					continue
				}
			}
		}
		out = append(out, tokens[i])
	}
	return out
}

// translateCurrentDatetime converts CURRENT_DATE -> date('now'), CURRENT_TIME -> time('now'),
// CURRENT_TIMESTAMP -> datetime('now').
func translateCurrentDatetime(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t.Kind == TokKeyword {
			switch t.Value {
			case "CURRENT_DATE":
				out = append(out,
					Token{Kind: TokIdent, Value: "date", Raw: "date"},
					Token{Kind: TokParen, Value: "(", Raw: "("},
					Token{Kind: TokString, Value: "'now'", Raw: "'now'"},
					Token{Kind: TokParen, Value: ")", Raw: ")"},
				)
				continue
			case "CURRENT_TIME":
				out = append(out,
					Token{Kind: TokIdent, Value: "time", Raw: "time"},
					Token{Kind: TokParen, Value: "(", Raw: "("},
					Token{Kind: TokString, Value: "'now'", Raw: "'now'"},
					Token{Kind: TokParen, Value: ")", Raw: ")"},
				)
				continue
			case "CURRENT_TIMESTAMP":
				out = append(out,
					Token{Kind: TokIdent, Value: "datetime", Raw: "datetime"},
					Token{Kind: TokParen, Value: "(", Raw: "("},
					Token{Kind: TokString, Value: "'now'", Raw: "'now'"},
					Token{Kind: TokParen, Value: ")", Raw: ")"},
				)
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

// translateDateTrunc converts date_trunc('field', expr) to appropriate strftime call.
func translateDateTrunc(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind == TokIdent && strings.ToLower(tokens[i].Value) == "date_trunc" {
			// Look for (
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}
			if j < len(tokens) && tokens[j].Kind == TokParen && tokens[j].Value == "(" {
				// Parse arguments
				args, endIdx := parseFuncArgs(tokens, j)
				if len(args) == 2 {
					field := extractStringLiteral(args[0])
					exprTokens := args[1]

					replacement := dateTruncReplacement(field, exprTokens)
					if replacement != nil {
						out = append(out, replacement...)
						i = endIdx
						continue
					}
				}
			}
		}
		out = append(out, tokens[i])
	}
	return out
}

// dateTruncReplacement returns the SQLite replacement for date_trunc.
func dateTruncReplacement(field string, expr []Token) []Token {
	field = strings.ToLower(strings.Trim(field, "'"))
	var result []Token

	switch field {
	case "day":
		// date(expr)
		result = append(result, Token{Kind: TokIdent, Value: "date", Raw: "date"})
		result = append(result, Token{Kind: TokParen, Value: "(", Raw: "("})
		result = append(result, expr...)
		result = append(result, Token{Kind: TokParen, Value: ")", Raw: ")"})
	case "hour":
		result = strftimeCall("'%Y-%m-%d %H:00:00'", expr)
	case "minute":
		result = strftimeCall("'%Y-%m-%d %H:%M:00'", expr)
	case "month":
		result = strftimeCall("'%Y-%m-01'", expr)
	case "year":
		result = strftimeCall("'%Y-01-01'", expr)
	case "second":
		result = strftimeCall("'%Y-%m-%d %H:%M:%S'", expr)
	default:
		return nil
	}
	return result
}

// strftimeCall builds: strftime(fmt, expr)
func strftimeCall(format string, expr []Token) []Token {
	var result []Token
	result = append(result, Token{Kind: TokIdent, Value: "strftime", Raw: "strftime"})
	result = append(result, Token{Kind: TokParen, Value: "(", Raw: "("})
	result = append(result, Token{Kind: TokString, Value: format, Raw: format})
	result = append(result, Token{Kind: TokComma, Value: ",", Raw: ","})
	result = append(result, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
	result = append(result, expr...)
	result = append(result, Token{Kind: TokParen, Value: ")", Raw: ")"})
	return result
}

// translateExtract converts EXTRACT(field FROM expr) to CAST(strftime(fmt, expr) AS INTEGER).
func translateExtract(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind == TokKeyword && tokens[i].Value == "EXTRACT" {
			// Look for (
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}
			if j < len(tokens) && tokens[j].Kind == TokParen && tokens[j].Value == "(" {
				// Parse: field FROM expr
				k := j + 1
				for k < len(tokens) && tokens[k].Kind == TokWhitespace {
					k++
				}
				// Read field name
				if k < len(tokens) && (tokens[k].Kind == TokKeyword || tokens[k].Kind == TokIdent) {
					field := strings.ToLower(tokens[k].Value)
					l := k + 1
					for l < len(tokens) && tokens[l].Kind == TokWhitespace {
						l++
					}
					if l < len(tokens) && tokens[l].Kind == TokKeyword && tokens[l].Value == "FROM" {
						// Read expr until closing )
						m := l + 1
						for m < len(tokens) && tokens[m].Kind == TokWhitespace {
							m++
						}
						exprStart := m
						depth := 1
						for m < len(tokens) && depth > 0 {
							if tokens[m].Kind == TokParen && tokens[m].Value == "(" {
								depth++
							} else if tokens[m].Kind == TokParen && tokens[m].Value == ")" {
								depth--
								if depth == 0 {
									break
								}
							}
							m++
						}
						exprTokens := tokens[exprStart:m]
						// Trim trailing whitespace
						for len(exprTokens) > 0 && exprTokens[len(exprTokens)-1].Kind == TokWhitespace {
							exprTokens = exprTokens[:len(exprTokens)-1]
						}

						fmt := extractFieldFormat(field)
						if fmt != "" {
							// CAST(strftime(fmt, expr) AS INTEGER)
							out = append(out, Token{Kind: TokKeyword, Value: "CAST", Raw: "CAST"})
							out = append(out, Token{Kind: TokParen, Value: "(", Raw: "("})
							out = append(out, strftimeCall("'"+fmt+"'", exprTokens)...)
							out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
							out = append(out, Token{Kind: TokKeyword, Value: "AS", Raw: "AS"})
							out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
							out = append(out, Token{Kind: TokKeyword, Value: "INTEGER", Raw: "INTEGER"})
							out = append(out, Token{Kind: TokParen, Value: ")", Raw: ")"})
							i = m
							continue
						}
					}
				}
			}
		}
		out = append(out, tokens[i])
	}
	return out
}

// extractFieldFormat returns the strftime format string for an EXTRACT field.
func extractFieldFormat(field string) string {
	switch field {
	case "year":
		return "%Y"
	case "month":
		return "%m"
	case "day":
		return "%d"
	case "hour":
		return "%H"
	case "minute":
		return "%M"
	case "second":
		return "%S"
	case "dow", "dayofweek":
		return "%w"
	case "doy", "dayofyear":
		return "%j"
	}
	return ""
}

// translateStringFuncs handles string function rewrites.
func translateStringFuncs(tokens []Token) []Token {
	tokens = translateLeftRight(tokens)
	tokens = translateConcat(tokens)
	return tokens
}

// translateLeftRight converts left(str, n) -> substr(str, 1, n) and right(str, n) -> substr(str, -n).
func translateLeftRight(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind == TokIdent || tokens[i].Kind == TokKeyword {
			lower := strings.ToLower(tokens[i].Value)
			if lower == "left" || lower == "right" {
				j := i + 1
				for j < len(tokens) && tokens[j].Kind == TokWhitespace {
					j++
				}
				if j < len(tokens) && tokens[j].Kind == TokParen && tokens[j].Value == "(" {
					args, endIdx := parseFuncArgs(tokens, j)
					if len(args) == 2 {
						out = append(out, Token{Kind: TokIdent, Value: "substr", Raw: "substr"})
						out = append(out, Token{Kind: TokParen, Value: "(", Raw: "("})
						out = append(out, args[0]...)
						out = append(out, Token{Kind: TokComma, Value: ",", Raw: ","})
						out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
						if lower == "left" {
							out = append(out, Token{Kind: TokNumber, Value: "1", Raw: "1"})
							out = append(out, Token{Kind: TokComma, Value: ",", Raw: ","})
							out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
							out = append(out, args[1]...)
						} else {
							out = append(out, Token{Kind: TokOperator, Value: "-", Raw: "-"})
							out = append(out, args[1]...)
						}
						out = append(out, Token{Kind: TokParen, Value: ")", Raw: ")"})
						i = endIdx
						continue
					}
				}
			}
		}
		out = append(out, tokens[i])
	}
	return out
}

// translateConcat converts concat(a, b, ...) to (COALESCE(a,'') || COALESCE(b,'') || ...).
func translateConcat(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind == TokIdent && strings.ToLower(tokens[i].Value) == "concat" {
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}
			if j < len(tokens) && tokens[j].Kind == TokParen && tokens[j].Value == "(" {
				args, endIdx := parseFuncArgs(tokens, j)
				if len(args) > 0 {
					out = append(out, Token{Kind: TokParen, Value: "(", Raw: "("})
					for ai, arg := range args {
						if ai > 0 {
							out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
							out = append(out, Token{Kind: TokOperator, Value: "||", Raw: "||"})
							out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
						}
						out = append(out, Token{Kind: TokKeyword, Value: "COALESCE", Raw: "COALESCE"})
						out = append(out, Token{Kind: TokParen, Value: "(", Raw: "("})
						out = append(out, arg...)
						out = append(out, Token{Kind: TokComma, Value: ",", Raw: ","})
						out = append(out, Token{Kind: TokString, Value: "''", Raw: "''"})
						out = append(out, Token{Kind: TokParen, Value: ")", Raw: ")"})
					}
					out = append(out, Token{Kind: TokParen, Value: ")", Raw: ")"})
					i = endIdx
					continue
				}
			}
		}
		out = append(out, tokens[i])
	}
	return out
}

// translateAggFuncs converts string_agg -> group_concat, array_agg -> json_group_array.
func translateAggFuncs(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind == TokIdent {
			lower := strings.ToLower(tokens[i].Value)
			switch lower {
			case "string_agg":
				out = append(out, Token{Kind: TokIdent, Value: "group_concat", Raw: "group_concat"})
				continue
			case "array_agg":
				out = append(out, Token{Kind: TokIdent, Value: "json_group_array", Raw: "json_group_array"})
				continue
			case "date_part":
				// date_part('field', expr) -> CAST(strftime(fmt, expr) AS INTEGER)
				j := i + 1
				for j < len(tokens) && tokens[j].Kind == TokWhitespace {
					j++
				}
				if j < len(tokens) && tokens[j].Kind == TokParen && tokens[j].Value == "(" {
					args, endIdx := parseFuncArgs(tokens, j)
					if len(args) == 2 {
						field := strings.ToLower(strings.Trim(extractStringLiteral(args[0]), "'"))
						fmt := extractFieldFormat(field)
						if fmt != "" {
							out = append(out, Token{Kind: TokKeyword, Value: "CAST", Raw: "CAST"})
							out = append(out, Token{Kind: TokParen, Value: "(", Raw: "("})
							out = append(out, strftimeCall("'"+fmt+"'", args[1])...)
							out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
							out = append(out, Token{Kind: TokKeyword, Value: "AS", Raw: "AS"})
							out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
							out = append(out, Token{Kind: TokKeyword, Value: "INTEGER", Raw: "INTEGER"})
							out = append(out, Token{Kind: TokParen, Value: ")", Raw: ")"})
							i = endIdx
							continue
						}
					}
				}
			case "to_char":
				// to_char(expr, format) -> strftime(mapped_format, expr) or pg_to_char(expr, format)
				j := i + 1
				for j < len(tokens) && tokens[j].Kind == TokWhitespace {
					j++
				}
				if j < len(tokens) && tokens[j].Kind == TokParen && tokens[j].Value == "(" {
					args, endIdx := parseFuncArgs(tokens, j)
					if len(args) == 2 {
						pgFmt := extractStringLiteral(args[1])
						sqliteFmt, canMap := mapPGDateFormat(pgFmt)
						if canMap && sqliteFmt != "" {
							// Fast path: strftime
							out = append(out, Token{Kind: TokIdent, Value: "strftime", Raw: "strftime"})
							out = append(out, Token{Kind: TokParen, Value: "(", Raw: "("})
							out = append(out, Token{Kind: TokString, Value: "'" + sqliteFmt + "'", Raw: "'" + sqliteFmt + "'"})
							out = append(out, Token{Kind: TokComma, Value: ",", Raw: ","})
							out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
							out = append(out, args[0]...)
							out = append(out, Token{Kind: TokParen, Value: ")", Raw: ")"})
						} else {
							// Runtime fallback: pg_to_char
							out = append(out, Token{Kind: TokIdent, Value: "pg_to_char", Raw: "pg_to_char"})
							out = append(out, Token{Kind: TokParen, Value: "(", Raw: "("})
							out = append(out, args[0]...)
							out = append(out, Token{Kind: TokComma, Value: ",", Raw: ","})
							out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
							out = append(out, args[1]...)
							out = append(out, Token{Kind: TokParen, Value: ")", Raw: ")"})
						}
						i = endIdx
						continue
					}
				}
			}
		}
		out = append(out, tokens[i])
	}
	return out
}

// parseFuncArgs parses function arguments from an open paren.
// Returns a slice of token slices (one per arg) and the index of the closing paren.
func parseFuncArgs(tokens []Token, openParen int) ([][]Token, int) {
	var args [][]Token
	var current []Token
	depth := 0
	i := openParen

	for i < len(tokens) {
		t := tokens[i]
		if t.Kind == TokParen && t.Value == "(" {
			depth++
			if depth == 1 {
				i++
				continue // skip the opening paren
			}
		}
		if t.Kind == TokParen && t.Value == ")" {
			depth--
			if depth == 0 {
				// Trim whitespace from current arg
				current = trimTokenWhitespace(current)
				if len(current) > 0 {
					args = append(args, current)
				}
				return args, i
			}
		}
		if t.Kind == TokComma && depth == 1 {
			current = trimTokenWhitespace(current)
			args = append(args, current)
			current = nil
			i++
			continue
		}
		current = append(current, t)
		i++
	}
	return args, i - 1
}

// trimTokenWhitespace removes leading and trailing whitespace tokens.
func trimTokenWhitespace(tokens []Token) []Token {
	for len(tokens) > 0 && tokens[0].Kind == TokWhitespace {
		tokens = tokens[1:]
	}
	for len(tokens) > 0 && tokens[len(tokens)-1].Kind == TokWhitespace {
		tokens = tokens[:len(tokens)-1]
	}
	return tokens
}

// extractStringLiteral extracts the value from string literal tokens.
func extractStringLiteral(tokens []Token) string {
	for _, t := range tokens {
		if t.Kind == TokString {
			return t.Value
		}
	}
	return ""
}

// mapPGDateFormat maps PostgreSQL date format strings to SQLite strftime formats.
// Returns the mapped format and true if all patterns can be mapped to strftime,
// or empty string and false if runtime fallback is needed.
func mapPGDateFormat(pgFmt string) (string, bool) {
	pgFmt = strings.Trim(pgFmt, "'")

	// Patterns that require runtime fallback (can't be mapped to strftime)
	runtimePatterns := []string{
		"Mon", "Month", "mon", "month", "MON", "MONTH",
		"Day", "Dy", "day", "dy", "DAY", "DY",
		"AM", "PM", "am", "pm", "A.M.", "P.M.",
		"TZ", "tz", "OF",
		"Q", // quarter
		"TM",
	}
	for _, p := range runtimePatterns {
		if strings.Contains(pgFmt, p) {
			return "", false
		}
	}

	replacer := strings.NewReplacer(
		"YYYY", "%Y",
		"YY", "%y",
		"MM", "%m",
		"DD", "%d",
		"HH24", "%H",
		"HH12", "%I",
		"HH", "%H",
		"MI", "%M",
		"SS", "%S",
	)
	return replacer.Replace(pgFmt), true
}
