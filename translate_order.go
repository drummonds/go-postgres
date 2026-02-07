package pglike

// translateNullsOrdering rewrites NULLS FIRST / NULLS LAST in ORDER BY clauses.
// "ORDER BY col [ASC|DESC] NULLS FIRST" ->
// "ORDER BY (CASE WHEN col IS NULL THEN 0 ELSE 1 END), col [ASC|DESC]"
// NULLS LAST uses THEN 1 ELSE 0.
func translateNullsOrdering(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		// Look for NULLS keyword
		if tokens[i].Kind != TokKeyword || tokens[i].Value != "NULLS" {
			out = append(out, tokens[i])
			continue
		}

		// Look ahead for FIRST or LAST
		j := i + 1
		for j < len(tokens) && tokens[j].Kind == TokWhitespace {
			j++
		}
		if j >= len(tokens) || tokens[j].Kind != TokKeyword || (tokens[j].Value != "FIRST" && tokens[j].Value != "LAST") {
			out = append(out, tokens[i])
			continue
		}

		nullsFirst := tokens[j].Value == "FIRST"

		// Walk backwards in out to find the column expression and optional ASC/DESC.
		// Pattern: ... col [ws] [ASC|DESC] [ws] <- current position
		pos := len(out)

		// Skip trailing whitespace
		for pos > 0 && out[pos-1].Kind == TokWhitespace {
			pos--
		}

		// Check for ASC/DESC
		var dirToken *Token
		dirPos := pos
		if pos > 0 && out[pos-1].Kind == TokKeyword && (out[pos-1].Value == "ASC" || out[pos-1].Value == "DESC") {
			t := out[pos-1]
			dirToken = &t
			pos--
			// Skip whitespace before ASC/DESC
			for pos > 0 && out[pos-1].Kind == TokWhitespace {
				pos--
			}
		}

		// The token at pos-1 should be the column name
		if pos == 0 {
			out = append(out, tokens[i])
			continue
		}
		colToken := out[pos-1]
		pos--

		// Truncate out to before the column
		out = out[:pos]

		// Determine CASE values
		thenVal := "0"
		elseVal := "1"
		if !nullsFirst {
			thenVal = "1"
			elseVal = "0"
		}

		// Emit: (CASE WHEN col IS NULL THEN X ELSE Y END), col [ASC|DESC]
		out = append(out,
			Token{Kind: TokParen, Value: "(", Raw: "("},
			Token{Kind: TokKeyword, Value: "CASE", Raw: "CASE"},
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
			Token{Kind: TokKeyword, Value: "WHEN", Raw: "WHEN"},
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
			colToken,
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
			Token{Kind: TokKeyword, Value: "IS", Raw: "IS"},
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
			Token{Kind: TokKeyword, Value: "NULL", Raw: "NULL"},
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
			Token{Kind: TokKeyword, Value: "THEN", Raw: "THEN"},
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
			Token{Kind: TokNumber, Value: thenVal, Raw: thenVal},
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
			Token{Kind: TokKeyword, Value: "ELSE", Raw: "ELSE"},
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
			Token{Kind: TokNumber, Value: elseVal, Raw: elseVal},
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
			Token{Kind: TokKeyword, Value: "END", Raw: "END"},
			Token{Kind: TokParen, Value: ")", Raw: ")"},
			Token{Kind: TokComma, Value: ",", Raw: ","},
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
			colToken,
		)

		if dirToken != nil {
			out = append(out,
				Token{Kind: TokWhitespace, Value: " ", Raw: " "},
				*dirToken,
			)
			_ = dirPos // used implicitly via truncation
		}

		i = j // skip past FIRST/LAST
	}
	return out
}
