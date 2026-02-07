package pglike

import "strings"

// translateGenerateSeries rewrites generate_series(start, stop[, step]) in FROM clause
// to a WITH RECURSIVE CTE that SQLite can evaluate.
//
// Input:  SELECT ... FROM generate_series(start, stop[, step]) [AS alias]
// Output: WITH RECURSIVE _gs(value) AS (
//
//	SELECT start UNION ALL SELECT value + step FROM _gs WHERE value + step <= stop
//
// ) SELECT ... FROM _gs [AS alias]
func translateGenerateSeries(tokens []Token) []Token {
	// Find: FROM generate_series(...)
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind != TokKeyword || tokens[i].Value != "FROM" {
			continue
		}

		// Skip whitespace after FROM
		j := i + 1
		for j < len(tokens) && tokens[j].Kind == TokWhitespace {
			j++
		}

		// Check for generate_series identifier
		if j >= len(tokens) || tokens[j].Kind != TokIdent || strings.ToLower(tokens[j].Value) != "generate_series" {
			continue
		}

		// Find opening paren
		k := j + 1
		for k < len(tokens) && tokens[k].Kind == TokWhitespace {
			k++
		}
		if k >= len(tokens) || tokens[k].Kind != TokParen || tokens[k].Value != "(" {
			continue
		}

		// Parse arguments
		args, endParen := parseFuncArgs(tokens, k)
		if len(args) < 2 || len(args) > 3 {
			continue
		}

		startTokens := args[0]
		stopTokens := args[1]
		stepStr := "1"
		if len(args) == 3 {
			stepStr = Reassemble(args[2])
		}

		startStr := Reassemble(startTokens)
		stopStr := Reassemble(stopTokens)

		// Collect any alias after the closing paren: [AS alias]
		aliasTokens := collectAlias(tokens, endParen+1)
		aliasEnd := endParen
		if len(aliasTokens) > 0 {
			aliasEnd = endParen + len(aliasTokens)
		}

		// Build: WITH RECURSIVE _gs(value) AS (SELECT start UNION ALL SELECT value + step FROM _gs WHERE value + step <= stop)
		cte := "WITH RECURSIVE _gs(value) AS (" +
			"SELECT " + startStr +
			" UNION ALL SELECT value + " + stepStr +
			" FROM _gs WHERE value + " + stepStr + " <= " + stopStr + ") "

		cteTokens := Tokenize(cte)

		// Build replacement: everything before FROM + CTE + everything after generate_series(...) [alias]
		var out []Token
		out = append(out, cteTokens...)
		out = append(out, tokens[:i]...) // everything before FROM
		out = append(out,
			Token{Kind: TokKeyword, Value: "FROM", Raw: "FROM"},
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
			Token{Kind: TokIdent, Value: "_gs", Raw: "_gs"},
		)

		// Append alias if present
		if len(aliasTokens) > 0 {
			out = append(out, aliasTokens...)
		}

		// Append rest of query after generate_series(...) [alias]
		if aliasEnd+1 < len(tokens) {
			out = append(out, tokens[aliasEnd+1:]...)
		}

		return out
	}
	return tokens
}

// collectAlias collects optional [ws] AS [ws] alias tokens starting at pos.
// Returns the collected tokens (including whitespace and AS).
func collectAlias(tokens []Token, pos int) []Token {
	i := pos
	var collected []Token

	// Skip whitespace
	for i < len(tokens) && tokens[i].Kind == TokWhitespace {
		collected = append(collected, tokens[i])
		i++
	}

	if i >= len(tokens) {
		return nil
	}

	// Check for AS keyword
	if tokens[i].Kind == TokKeyword && tokens[i].Value == "AS" {
		collected = append(collected, tokens[i])
		i++
		// Skip whitespace after AS
		for i < len(tokens) && tokens[i].Kind == TokWhitespace {
			collected = append(collected, tokens[i])
			i++
		}
		// Alias name
		if i < len(tokens) && (tokens[i].Kind == TokIdent || tokens[i].Kind == TokKeyword) {
			collected = append(collected, tokens[i])
			return collected
		}
	}

	// Check for bare alias (ident right after closing paren, no AS)
	if tokens[i].Kind == TokIdent {
		collected = append(collected, tokens[i])
		return collected
	}

	return nil
}
