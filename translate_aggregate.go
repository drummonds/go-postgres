package pglike

import (
	"database/sql/driver"
	"strings"
)

// numericAggregates maps a PG aggregate to the exact pg_numeric_* aggregate
// registered in numeric.go. count is not here: it never touches the value.
var numericAggregates = map[string]string{
	"sum": "pg_numeric_sum",
	"avg": "pg_numeric_avg",
	"min": "pg_numeric_min",
	"max": "pg_numeric_max",
}

// hasAggregateCall is the cheap pre-check before tokenizing a translated
// statement again for rewriteNumericAggregates.
func hasAggregateCall(sql string) bool {
	lower := strings.ToLower(sql)
	for name := range numericAggregates {
		if strings.Contains(lower, name) {
			return true
		}
	}
	return false
}

// rewriteNumericAggregates rewrites sum/avg/min/max over a bare column
// reference (col, t.col, s.t.col) to the exact pg_numeric_* aggregate when
// that column is declared NUMERIC in a table the statement names. A bare
// NUMERIC column is TEXT in SQLite, so the built-in aggregates would sum
// in REAL and compare lexicographically. numeric maps a stored table name
// (lower case) to its NUMERIC column names (lower case). Anything else,
// including an expression argument, is left as it is: a ::numeric
// expression is already rewritten by translateNumeric.
func rewriteNumericAggregates(sql string, numeric map[string]map[string]bool) string {
	tokens := Tokenize(sql)
	var referenced []map[string]bool
	for _, t := range tokens {
		if t.Kind != TokIdent {
			continue
		}
		if cols, ok := numeric[strings.ToLower(unquoteIdent(t.Raw))]; ok {
			referenced = append(referenced, cols)
		}
	}
	if len(referenced) == 0 {
		return sql
	}
	isNumeric := func(column string) bool {
		column = strings.ToLower(column)
		for _, cols := range referenced {
			if cols[column] {
				return true
			}
		}
		return false
	}
	changed := false
	for i, t := range tokens {
		if t.Kind != TokIdent {
			continue
		}
		agg, ok := numericAggregates[strings.ToLower(t.Value)]
		if !ok {
			continue
		}
		column, ok := bareColumnArgument(tokens, i+1)
		if !ok || !isNumeric(column) {
			continue
		}
		tokens[i] = Token{Kind: TokIdent, Value: agg, Raw: agg}
		changed = true
	}
	if !changed {
		return sql
	}
	return Reassemble(tokens)
}

// bareColumnArgument reports the column named by "( ident (. ident)* )"
// starting at tokens[i], skipping whitespace.
func bareColumnArgument(tokens []Token, i int) (string, bool) {
	next := func() (Token, bool) {
		for ; i < len(tokens); i++ {
			if tokens[i].Kind != TokWhitespace && tokens[i].Kind != TokComment {
				t := tokens[i]
				i++
				return t, true
			}
		}
		return Token{}, false
	}
	t, ok := next()
	if !ok || t.Kind != TokParen || t.Value != "(" {
		return "", false
	}
	column := ""
	for {
		t, ok = next()
		if !ok || t.Kind != TokIdent {
			return "", false
		}
		column = unquoteIdent(t.Raw)
		t, ok = next()
		if !ok {
			return "", false
		}
		switch {
		case t.Kind == TokParen && t.Value == ")":
			return column, true
		case t.Kind == TokDot:
			continue
		default:
			return "", false
		}
	}
}

// numericColumns returns the NUMERIC columns of every table, read from the
// /*pg:numeric*/ annotations in sqlite_master and cached until the schema
// version changes.
func (c *conn) numericColumns() (map[string]map[string]bool, error) {
	version, err := c.queryDirectInt64("PRAGMA schema_version")
	if err != nil {
		return nil, err
	}
	if c.numericCols != nil && version == c.numericSchemaVersion {
		return c.numericCols, nil
	}
	cols := map[string]map[string]bool{}
	s, err := c.inner.Prepare("SELECT name, sql FROM sqlite_master WHERE type = 'table' AND sql LIKE '%" + pgTypeCommentPrefix + "numeric*/%'")
	if err != nil {
		return nil, err
	}
	defer s.Close()
	r, err := s.Query(nil) //nolint:staticcheck
	if err != nil {
		return nil, err
	}
	defer r.Close()
	_ = r.Columns()
	dest := make([]driver.Value, 2)
	for {
		if err := r.Next(dest); err != nil {
			break
		}
		name, _ := dest[0].(string)
		tableSQL, _ := dest[1].(string)
		set := map[string]bool{}
		for _, col := range annotatedNumericColumns(tableSQL) {
			set[strings.ToLower(col)] = true
		}
		cols[strings.ToLower(name)] = set
	}
	c.numericCols, c.numericSchemaVersion = cols, version
	return cols, nil
}

// annotatedNumericColumns lists the columns of a translated CREATE TABLE
// whose type carries the /*pg:numeric*/ annotation.
func annotatedNumericColumns(tableSQL string) []string {
	var out []string
	depth := 0
	atDefStart := false
	column := ""
	for _, t := range Tokenize(tableSQL) {
		switch {
		case t.Kind == TokParen && t.Value == "(":
			depth++
			if depth == 1 {
				atDefStart = true
			}
			continue
		case t.Kind == TokParen && t.Value == ")":
			depth--
			continue
		case t.Kind == TokComma && depth == 1:
			atDefStart, column = true, ""
			continue
		case t.Kind == TokWhitespace:
			continue
		}
		if depth != 1 {
			continue
		}
		if atDefStart {
			atDefStart = false
			column = unquoteIdent(t.Raw)
			continue
		}
		if column != "" && t.Kind == TokComment && t.Raw == pgTypeCommentPrefix+"numeric*/" {
			out = append(out, column)
			column = ""
		}
	}
	return out
}
