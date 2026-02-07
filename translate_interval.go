package pglike

import "strings"

// translateInterval rewrites expr +/- INTERVAL 'N unit' to datetime(expr, '+/-N unit').
// Also handles the INTERVAL '1' DAY syntax (unit as separate keyword).
func translateInterval(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); i++ {
		// Look for + or - operator
		if tokens[i].Kind == TokOperator && (tokens[i].Value == "+" || tokens[i].Value == "-") {
			op := tokens[i].Value

			// Look ahead for INTERVAL keyword
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokWhitespace {
				j++
			}
			if j < len(tokens) && tokens[j].Kind == TokKeyword && tokens[j].Value == "INTERVAL" {
				// Look for the interval value string
				k := j + 1
				for k < len(tokens) && tokens[k].Kind == TokWhitespace {
					k++
				}
				if k < len(tokens) && tokens[k].Kind == TokString {
					intervalStr := strings.Trim(tokens[k].Value, "'")
					endIdx := k

					// Check for INTERVAL '1' DAY syntax (unit as separate keyword after the string)
					m := k + 1
					for m < len(tokens) && tokens[m].Kind == TokWhitespace {
						m++
					}
					if m < len(tokens) && (tokens[m].Kind == TokKeyword || tokens[m].Kind == TokIdent) {
						unit := strings.ToLower(tokens[m].Value)
						if isIntervalUnit(unit) {
							intervalStr = intervalStr + " " + unit
							endIdx = m
						}
					}

					// Extract the left-hand expression from out (skip trailing whitespace)
					lhsEnd := len(out)
					for lhsEnd > 0 && out[lhsEnd-1].Kind == TokWhitespace {
						lhsEnd--
					}
					if lhsEnd == 0 {
						out = append(out, tokens[i])
						continue
					}

					// Collect the LHS expression tokens.
					// For simple cases: single ident/string/number or a function call like datetime('now')
					lhsTokens := extractLeftExpr(out[:lhsEnd])
					lhsCopy := make([]Token, len(lhsTokens))
					copy(lhsCopy, lhsTokens)
					out = out[:lhsEnd-len(lhsTokens)]

					// Build modifier string: +/-N unit
					sign := "+"
					if op == "-" {
						sign = "-"
					}
					modifier := sign + intervalStr

					// Emit: datetime(lhs, 'modifier')
					out = append(out,
						Token{Kind: TokIdent, Value: "datetime", Raw: "datetime"},
						Token{Kind: TokParen, Value: "(", Raw: "("},
					)
					out = append(out, lhsCopy...)
					out = append(out,
						Token{Kind: TokComma, Value: ",", Raw: ","},
						Token{Kind: TokWhitespace, Value: " ", Raw: " "},
						Token{Kind: TokString, Value: "'" + modifier + "'", Raw: "'" + modifier + "'"},
						Token{Kind: TokParen, Value: ")", Raw: ")"},
					)
					i = endIdx
					continue
				}
			}
		}
		out = append(out, tokens[i])
	}
	return out
}

// isIntervalUnit checks if a keyword is a valid interval unit.
func isIntervalUnit(s string) bool {
	switch s {
	case "year", "years", "month", "months", "day", "days",
		"hour", "hours", "minute", "minutes", "second", "seconds":
		return true
	}
	return false
}
