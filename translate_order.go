package pglike

// translateNullsOrdering rewrites NULLS FIRST / NULLS LAST in ORDER BY clauses.
// "ORDER BY col [ASC|DESC] NULLS FIRST" ->
// "ORDER BY (CASE WHEN col IS NULL THEN 0 ELSE 1 END), col [ASC|DESC]"
// NULLS LAST uses THEN 1 ELSE 0.
// Handles simple identifiers, table-qualified names (t.col), and expressions (LOWER(name)).
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
		pos := len(out)

		// Skip trailing whitespace
		for pos > 0 && out[pos-1].Kind == TokWhitespace {
			pos--
		}

		// Check for ASC/DESC
		var dirToken *Token
		if pos > 0 && out[pos-1].Kind == TokKeyword && (out[pos-1].Value == "ASC" || out[pos-1].Value == "DESC") {
			t := out[pos-1]
			dirToken = &t
			pos--
			// Skip whitespace before ASC/DESC
			for pos > 0 && out[pos-1].Kind == TokWhitespace {
				pos--
			}
		}

		// Extract the column expression by walking backwards.
		colEnd := pos
		colStart := findColumnExprStart(out, pos)
		if colStart == colEnd {
			out = append(out, tokens[i])
			continue
		}

		// Copy the column expression tokens (avoid slice aliasing).
		colTokens := make([]Token, colEnd-colStart)
		copy(colTokens, out[colStart:colEnd])

		// Truncate out to before the column expression
		out = out[:colStart]

		// Determine CASE values
		thenVal := "0"
		elseVal := "1"
		if !nullsFirst {
			thenVal = "1"
			elseVal = "0"
		}

		// Emit: (CASE WHEN <col> IS NULL THEN X ELSE Y END), <col> [ASC|DESC]
		out = append(out,
			Token{Kind: TokParen, Value: "(", Raw: "("},
			Token{Kind: TokKeyword, Value: "CASE", Raw: "CASE"},
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
			Token{Kind: TokKeyword, Value: "WHEN", Raw: "WHEN"},
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
		)
		out = append(out, colTokens...)
		out = append(out,
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
		)
		out = append(out, colTokens...)

		if dirToken != nil {
			out = append(out,
				Token{Kind: TokWhitespace, Value: " ", Raw: " "},
				*dirToken,
			)
		}

		i = j // skip past FIRST/LAST
	}
	return out
}

// findColumnExprStart walks backwards from pos to find the start of a column expression.
// Handles: simple identifiers, table.column, and function calls like LOWER(name).
func findColumnExprStart(tokens []Token, pos int) int {
	if pos == 0 {
		return pos
	}

	end := pos - 1

	// If the expression ends with ), walk backwards past the matched parentheses
	if tokens[end].Kind == TokParen && tokens[end].Value == ")" {
		depth := 1
		p := end - 1
		for p >= 0 && depth > 0 {
			if tokens[p].Kind == TokParen && tokens[p].Value == ")" {
				depth++
			} else if tokens[p].Kind == TokParen && tokens[p].Value == "(" {
				depth--
			}
			if depth > 0 {
				p--
			}
		}
		// p is now at the opening paren; the function name is before it
		if p > 0 && (tokens[p-1].Kind == TokIdent || tokens[p-1].Kind == TokKeyword) {
			return p - 1
		}
		return p
	}

	// Simple identifier or keyword
	if tokens[end].Kind == TokIdent || tokens[end].Kind == TokKeyword {
		start := end
		// Check for table.column pattern: walk back past .table
		if start >= 2 && tokens[start-1].Kind == TokDot && (tokens[start-2].Kind == TokIdent || tokens[start-2].Kind == TokKeyword) {
			start -= 2
		}
		return start
	}

	return pos
}
