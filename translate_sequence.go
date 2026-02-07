package pglike

import "fmt"

// translateSequenceDDL translates CREATE SEQUENCE and DROP SEQUENCE statements.
// CREATE SEQUENCE name [INCREMENT BY n] [START WITH n] ->
//
//	INSERT OR IGNORE INTO _sequences (name, current_value, increment) VALUES ('name', startVal-1, increment)
//
// DROP SEQUENCE name -> DELETE FROM _sequences WHERE name = 'name'
func translateSequenceDDL(tokens []Token) []Token {
	// Look for CREATE SEQUENCE or DROP SEQUENCE
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind != TokKeyword {
			continue
		}

		switch tokens[i].Value {
		case "CREATE":
			if result, ok := translateCreateSequence(tokens, i); ok {
				return result
			}
		case "DROP":
			if result, ok := translateDropSequence(tokens, i); ok {
				return result
			}
		}
	}
	return tokens
}

func translateCreateSequence(tokens []Token, start int) ([]Token, bool) {
	// CREATE [ws] SEQUENCE [ws] name [options...]
	j := start + 1
	for j < len(tokens) && tokens[j].Kind == TokWhitespace {
		j++
	}
	if j >= len(tokens) || tokens[j].Kind != TokKeyword || tokens[j].Value != "SEQUENCE" {
		return nil, false
	}

	// Skip to sequence name
	k := j + 1
	for k < len(tokens) && tokens[k].Kind == TokWhitespace {
		k++
	}
	if k >= len(tokens) || (tokens[k].Kind != TokIdent && tokens[k].Kind != TokKeyword) {
		return nil, false
	}
	seqName := tokens[k].Value

	// Parse options: INCREMENT BY n, START WITH n
	increment := 1
	startVal := 0
	m := k + 1
	for m < len(tokens) {
		if tokens[m].Kind == TokWhitespace || tokens[m].Kind == TokSemicolon {
			m++
			continue
		}
		if tokens[m].Kind == TokKeyword {
			switch tokens[m].Value {
			case "INCREMENT":
				// INCREMENT [BY] n
				m++
				for m < len(tokens) && tokens[m].Kind == TokWhitespace {
					m++
				}
				if m < len(tokens) && tokens[m].Kind == TokKeyword && tokens[m].Value == "BY" {
					m++
					for m < len(tokens) && tokens[m].Kind == TokWhitespace {
						m++
					}
				}
				if m < len(tokens) && tokens[m].Kind == TokNumber {
					fmt.Sscanf(tokens[m].Value, "%d", &increment)
					m++
				}
			case "START":
				// START [WITH] n
				m++
				for m < len(tokens) && tokens[m].Kind == TokWhitespace {
					m++
				}
				if m < len(tokens) && tokens[m].Kind == TokKeyword && (tokens[m].Value == "WITH" || tokens[m].Value == "AS") {
					m++
					for m < len(tokens) && tokens[m].Kind == TokWhitespace {
						m++
					}
				}
				if m < len(tokens) && tokens[m].Kind == TokNumber {
					fmt.Sscanf(tokens[m].Value, "%d", &startVal)
					m++
				}
			default:
				m++ // skip unknown options (MINVALUE, MAXVALUE, CYCLE, etc.)
			}
		} else {
			m++
		}
	}

	// current_value is startVal - increment so first nextval returns startVal
	// If startVal is 0 (default), first nextval returns 0 + increment = 1
	currentValue := startVal - increment
	if startVal == 0 {
		currentValue = 0
	}

	sql := fmt.Sprintf("INSERT OR IGNORE INTO _sequences (name, current_value, increment) VALUES ('%s', %d, %d)",
		seqName, currentValue, increment)
	return Tokenize(sql), true
}

func translateDropSequence(tokens []Token, start int) ([]Token, bool) {
	// DROP [ws] SEQUENCE [ws] name
	j := start + 1
	for j < len(tokens) && tokens[j].Kind == TokWhitespace {
		j++
	}
	if j >= len(tokens) || tokens[j].Kind != TokKeyword || tokens[j].Value != "SEQUENCE" {
		return nil, false
	}

	k := j + 1
	for k < len(tokens) && tokens[k].Kind == TokWhitespace {
		k++
	}

	// Handle IF EXISTS
	if k < len(tokens) && tokens[k].Kind == TokKeyword && tokens[k].Value == "IF" {
		k++
		for k < len(tokens) && tokens[k].Kind == TokWhitespace {
			k++
		}
		if k < len(tokens) && tokens[k].Kind == TokKeyword && tokens[k].Value == "EXISTS" {
			k++
			for k < len(tokens) && tokens[k].Kind == TokWhitespace {
				k++
			}
		}
	}

	if k >= len(tokens) || (tokens[k].Kind != TokIdent && tokens[k].Kind != TokKeyword) {
		return nil, false
	}
	seqName := tokens[k].Value

	sql := fmt.Sprintf("DELETE FROM _sequences WHERE name = '%s'", seqName)
	return Tokenize(sql), true
}
