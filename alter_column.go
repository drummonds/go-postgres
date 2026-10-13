package pglike

import (
	"database/sql/driver"
	"errors"
	"strings"
)

// alterColumnType is a parsed
//
//	ALTER TABLE [ONLY] table ALTER [COLUMN] column [SET DATA] TYPE type [USING expr]
//
// PostgreSQL retypes the column in place. SQLite has no such statement, so
// the driver rebuilds the table: its rows are kept in a temporary copy, the
// table is dropped and created again with the column's new type, the rows
// are put back through the USING expression, and its indexes and triggers
// are recreated. Views over the table are untouched and read the rebuilt
// table by name. Foreign keys that reference the table are deferred for
// the transaction, so the drop and the reinsert net to no violation.
type alterColumnType struct {
	table    string
	column   string
	typeSQL  string // the PG type as written
	usingSQL string // the USING expression as written; "" for none
}

// parseAlterColumnType recognises ALTER TABLE ... ALTER COLUMN ... TYPE.
func parseAlterColumnType(tokens []Token) (*alterColumnType, bool) {
	var words []Token
	var pos []int // index in tokens of each word
	for i, t := range tokens {
		if t.Kind != TokWhitespace && t.Kind != TokComment {
			words = append(words, t)
			pos = append(pos, i)
		}
	}
	is := func(i int, w string) bool {
		return i < len(words) && isNamePart(words[i]) && strings.EqualFold(words[i].Raw, w)
	}
	if !is(0, "ALTER") || !is(1, "TABLE") {
		return nil, false
	}
	i := 2
	if is(i, "ONLY") {
		i++
	}
	d := &alterColumnType{}
	table, next, ok := relationName(words, i)
	if !ok {
		return nil, false
	}
	d.table = table
	i = next
	if !is(i, "ALTER") {
		return nil, false
	}
	i++
	if is(i, "COLUMN") {
		i++
	}
	if i >= len(words) || !isNamePart(words[i]) {
		return nil, false
	}
	d.column = foldIdent(words[i])
	i++
	if is(i, "SET") && is(i+1, "DATA") {
		i += 2
	}
	if !is(i, "TYPE") {
		return nil, false
	}
	i++
	j := i
	for j < len(words) && !is(j, "USING") {
		j++
	}
	if j == i {
		return nil, false
	}
	// The type and the USING expression keep their original text, so a
	// cast such as id::uuid reaches the translator as written.
	typeEnd := len(tokens)
	if j < len(words) {
		if j+1 == len(words) {
			return nil, false
		}
		typeEnd = pos[j]
		d.usingSQL = strings.TrimSpace(Reassemble(tokens[pos[j+1]:]))
	}
	d.typeSQL = strings.TrimSpace(Reassemble(tokens[pos[i]:typeEnd]))
	return d, true
}

// relationName reads a [schema.]name at words[i] and returns the stored
// table name (see translateSchemas) and the index after it.
func relationName(words []Token, i int) (string, int, bool) {
	if i >= len(words) || !isNamePart(words[i]) {
		return "", i, false
	}
	name := foldIdent(words[i])
	i++
	if i+1 < len(words) && words[i].Kind == TokDot && isNamePart(words[i+1]) {
		if name != "public" {
			name += "." + foldIdent(words[i+1])
		} else {
			name = foldIdent(words[i+1])
		}
		i += 2
	}
	return name, i, true
}

const alterColumnCopy = `"_pglike_alter_column"`

// execAlterColumnType rebuilds d.table with d.column retyped.
func (c *conn) execAlterColumnType(d *alterColumnType) (driver.Result, error) {
	names, err := c.queryDirectStrings(`SELECT name FROM sqlite_master WHERE type = 'table' AND lower(name) = lower(?)`, d.table)
	if err != nil {
		return nil, wrapError(err)
	}
	if len(names) == 0 {
		return nil, pgErrorf("42P01", `relation "%s" does not exist`, d.table)
	}
	name := names[0]
	quoted := quotedIdent(name).Raw
	sqls, err := c.queryDirectStrings(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, name)
	if err != nil {
		return nil, wrapError(err)
	}
	createSQL, ok := retypeColumnSQL(sqls[0], d)
	if !ok {
		return nil, pgErrorf("42703", `column "%s" of relation "%s" does not exist`, d.column, d.table)
	}
	deps, err := c.queryDirectStrings(`SELECT sql FROM sqlite_master WHERE type IN ('index', 'trigger') AND tbl_name = ? AND sql IS NOT NULL`, name)
	if err != nil {
		return nil, wrapError(err)
	}
	cols, err := c.queryDirectStrings(`SELECT name FROM pragma_table_info(?)`, name)
	if err != nil {
		return nil, wrapError(err)
	}
	copyBack, err := Translate(copyRowsSQL(quoted, cols, d))
	if err != nil {
		return nil, err
	}

	steps := []string{
		"SAVEPOINT pglike_alter_column",
		"PRAGMA defer_foreign_keys = ON",
		"CREATE TEMP TABLE " + alterColumnCopy + " AS SELECT * FROM " + quoted,
		"DROP TABLE " + quoted,
		createSQL,
		copyBack,
		"DROP TABLE " + alterColumnCopy,
	}
	steps = append(steps, deps...)
	steps = append(steps, "RELEASE pglike_alter_column")
	for _, s := range steps {
		if err := c.execDirect(s); err != nil {
			_ = c.execDirect("ROLLBACK TO pglike_alter_column")
			_ = c.execDirect("RELEASE pglike_alter_column")
			return nil, wrapError(err)
		}
	}
	return driver.ResultNoRows, nil
}

// copyRowsSQL is the PG-dialect INSERT ... SELECT that refills the rebuilt
// table from the copy, the retyped column through the USING expression.
func copyRowsSQL(into string, cols []string, d *alterColumnType) string {
	names := make([]string, len(cols))
	exprs := make([]string, len(cols))
	for i, col := range cols {
		names[i] = quotedIdent(col).Raw
		exprs[i] = names[i]
		if strings.EqualFold(col, d.column) && d.usingSQL != "" {
			exprs[i] = d.usingSQL
		}
	}
	return "INSERT INTO " + into + " (" + strings.Join(names, ", ") + ") SELECT " +
		strings.Join(exprs, ", ") + " FROM " + alterColumnCopy
}

// columnConstraintKeywords end a column's type in a column definition.
var columnConstraintKeywords = map[string]bool{
	"CONSTRAINT": true, "PRIMARY": true, "NOT": true, "NULL": true, "UNIQUE": true,
	"CHECK": true, "DEFAULT": true, "REFERENCES": true, "COLLATE": true,
	"GENERATED": true, "AS": true,
}

// retypeColumnSQL rewrites a stored CREATE TABLE statement with d.column's
// type replaced by the translation of d.typeSQL. It reports false when the
// column is not defined.
func retypeColumnSQL(tableSQL string, d *alterColumnType) (string, bool) {
	tokens := Tokenize(tableSQL)
	depth := 0
	atDefStart := false
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		switch {
		case t.Kind == TokWhitespace || t.Kind == TokComment:
			continue
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
			atDefStart = true
			continue
		}
		if depth != 1 || !atDefStart {
			continue
		}
		atDefStart = false
		if !isNamePart(t) || !strings.EqualFold(unquoteIdent(t.Raw), d.column) {
			continue
		}
		start, end := columnTypeSpan(tokens, i+1)
		newType := translateTypes(append([]Token{
			{Kind: TokParen, Value: "(", Raw: "("}, t, {Kind: TokWhitespace, Value: " ", Raw: " "},
		}, Tokenize(d.typeSQL)...), true)[3:]
		out := append([]Token{}, tokens[:start]...)
		out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
		out = append(out, newType...)
		out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
		out = append(out, tokens[end:]...)
		return Reassemble(out), true
	}
	return "", false
}

// columnTypeSpan finds the tokens of a column's type, starting after its
// name at tokens[from]: up to the first column constraint keyword, a
// comma, or the table's closing paren, then any extras the driver itself
// emitted for a UUID column (its collation and validation constraint).
func columnTypeSpan(tokens []Token, from int) (start, end int) {
	start = from
	depth := 0
	i := from
	for ; i < len(tokens); i++ {
		t := tokens[i]
		if t.Kind == TokParen && t.Value == "(" {
			depth++
			continue
		}
		if t.Kind == TokParen && t.Value == ")" {
			if depth == 0 {
				break
			}
			depth--
			continue
		}
		if depth > 0 {
			continue
		}
		if t.Kind == TokComma || (isNamePart(t) && columnConstraintKeywords[strings.ToUpper(t.Raw)]) {
			break
		}
	}
	return start, skipUUIDExtras(tokens, i)
}

// skipUUIDExtras steps over "COLLATE NOCASE CONSTRAINT "pglike_uuid_..."
// CHECK (...)" when the driver's UUID translation put it there.
func skipUUIDExtras(tokens []Token, i int) int {
	j := nextNonTrivial(tokens, i)
	if j == -1 || !isNamePart(tokens[j]) || !strings.EqualFold(tokens[j].Raw, "COLLATE") {
		return i
	}
	k := nextNonTrivial(tokens, j+1)
	if k == -1 || !isNamePart(tokens[k]) || !strings.EqualFold(tokens[k].Raw, "NOCASE") {
		return i
	}
	i = k + 1
	j = nextNonTrivial(tokens, i)
	if j == -1 || !isNamePart(tokens[j]) || !strings.EqualFold(tokens[j].Raw, "CONSTRAINT") {
		return i
	}
	k = nextNonTrivial(tokens, j+1)
	if k == -1 || !strings.HasPrefix(unquoteIdent(tokens[k].Raw), uuidConstraintPrefix) {
		return i
	}
	k = nextNonTrivial(tokens, k+1) // CHECK
	k = nextNonTrivial(tokens, k+1) // (
	if k == -1 || tokens[k].Kind != TokParen {
		return i
	}
	return skipParenGroup(tokens, k) + 1
}

// alterColumnStmt is a prepared ALTER TABLE ... ALTER COLUMN ... TYPE.
type alterColumnStmt struct {
	c   *conn
	ddl *alterColumnType
}

func (s *alterColumnStmt) Close() error  { return nil }
func (s *alterColumnStmt) NumInput() int { return 0 }
func (s *alterColumnStmt) Exec([]driver.Value) (driver.Result, error) {
	return s.c.execAlterColumnType(s.ddl)
}
func (s *alterColumnStmt) Query([]driver.Value) (driver.Rows, error) {
	return nil, errors.New("pglike: ALTER TABLE returns no rows")
}
