package pglike

import (
	"database/sql/driver"
	"errors"
	"strings"
)

// driverDDL is a statement the driver runs itself rather than as
// translated SQL: CREATE/DROP SCHEMA, and the ALTER TABLE forms SQLite
// has no statement for, which rebuild the table.
type driverDDL interface {
	exec(c *conn) (driver.Result, error)
}

// parseDriverDDL recognises the statements the driver runs itself.
func parseDriverDDL(tokens []Token) (driverDDL, bool) {
	if d, ok := parseSchemaDDL(tokens); ok {
		return d, true
	}
	if d, ok := parseAlterColumnType(tokens); ok {
		return d, true
	}
	if d, ok := parseDropConstraint(tokens); ok {
		return d, true
	}
	if d, ok := parseAddForeignKey(tokens); ok {
		return d, true
	}
	return nil, false
}

// ddlStmt is a prepared driverDDL.
type ddlStmt struct {
	c   *conn
	ddl driverDDL
}

func (s *ddlStmt) Close() error  { return nil }
func (s *ddlStmt) NumInput() int { return 0 }
func (s *ddlStmt) Exec([]driver.Value) (driver.Result, error) {
	return s.ddl.exec(s.c)
}
func (s *ddlStmt) Query([]driver.Value) (driver.Rows, error) {
	return nil, errors.New("pglike: DDL returns no rows")
}

// alterTableWords reads ALTER TABLE [ONLY] table and returns the stored
// table name and the index of the next word.
func alterTableWords(words []Token) (string, int, bool) {
	is := func(i int, w string) bool {
		return i < len(words) && isNamePart(words[i]) && strings.EqualFold(words[i].Raw, w)
	}
	if !is(0, "ALTER") || !is(1, "TABLE") {
		return "", 0, false
	}
	i := 2
	if is(i, "ONLY") {
		i++
	}
	return relationName(words, i)
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

// significant returns a statement's tokens without whitespace and
// comments, with each one's index in the original stream.
func significant(tokens []Token) (words []Token, pos []int) {
	for i, t := range tokens {
		if t.Kind != TokWhitespace && t.Kind != TokComment {
			words = append(words, t)
			pos = append(pos, i)
		}
	}
	return words, pos
}

// storedTable is a table as sqlite_master holds it.
type storedTable struct {
	name string // as stored, for quoting
	sql  string // its CREATE TABLE
}

// lookupTable finds a table by its PG-folded name, or 42P01.
func (c *conn) lookupTable(folded string) (storedTable, error) {
	names, err := c.queryDirectStrings(`SELECT name FROM sqlite_master WHERE type = 'table' AND lower(name) = lower(?)`, folded)
	if err != nil {
		return storedTable{}, wrapError(err)
	}
	if len(names) == 0 {
		return storedTable{}, pgErrorf("42P01", `relation "%s" does not exist`, folded)
	}
	sqls, err := c.queryDirectStrings(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, names[0])
	if err != nil {
		return storedTable{}, wrapError(err)
	}
	return storedTable{name: names[0], sql: sqls[0]}, nil
}

const alterTableCopy = `"_pglike_alter_table"`

// rebuildTable replaces t with a table created by createSQL (a CREATE
// TABLE of the same name), keeping its rows: they go to a temporary copy,
// the table is dropped and created again, and the rows are put back by
// copyBack, an INSERT ... SELECT FROM the copy already translated. The
// table's indexes and triggers are recreated. It runs in a savepoint,
// so it rolls back with an enclosing transaction, with foreign keys
// deferred so a referenced table nets to no violation; a rebuild that
// leaves a child row without a parent fails where the transaction ends.
func (c *conn) rebuildTable(t storedTable, createSQL, copyBack string) error {
	quoted := quotedIdent(t.name).Raw
	deps, err := c.queryDirectStrings(`SELECT sql FROM sqlite_master WHERE type IN ('index', 'trigger') AND tbl_name = ? AND sql IS NOT NULL`, t.name)
	if err != nil {
		return wrapError(err)
	}
	steps := []string{
		"SAVEPOINT pglike_alter_table",
		"PRAGMA defer_foreign_keys = ON",
		"CREATE TEMP TABLE " + alterTableCopy + " AS SELECT * FROM " + quoted,
		"DROP TABLE " + quoted,
		createSQL,
		copyBack,
		"DROP TABLE " + alterTableCopy,
	}
	steps = append(steps, deps...)
	steps = append(steps, "RELEASE pglike_alter_table")
	for _, s := range steps {
		if err := c.execDirect(s); err != nil {
			_ = c.execDirect("ROLLBACK TO pglike_alter_table")
			_ = c.execDirect("RELEASE pglike_alter_table")
			return wrapError(err)
		}
	}
	return nil
}

// copyBackSQL is the translated INSERT ... SELECT that refills a rebuilt
// table from the copy; exprs, by lower-cased column name, are the columns
// read through an expression instead of as themselves.
func (c *conn) copyBackSQL(t storedTable, exprs map[string]string) (string, error) {
	cols, err := c.queryDirectStrings(`SELECT name FROM pragma_table_info(?)`, t.name)
	if err != nil {
		return "", wrapError(err)
	}
	names := make([]string, len(cols))
	reads := make([]string, len(cols))
	for i, col := range cols {
		names[i] = quotedIdent(col).Raw
		reads[i] = names[i]
		if e, ok := exprs[strings.ToLower(col)]; ok {
			reads[i] = e
		}
	}
	return Translate("INSERT INTO " + quotedIdent(t.name).Raw + " (" + strings.Join(names, ", ") + ") SELECT " +
		strings.Join(reads, ", ") + " FROM " + alterTableCopy)
}

// ---- ALTER COLUMN ... TYPE ------------------------------------------------

// alterColumnType is a parsed
//
//	ALTER TABLE [ONLY] table ALTER [COLUMN] column [SET DATA] TYPE type [USING expr] [, ALTER [COLUMN] ...]
//
// PostgreSQL retypes the columns in place, in one pass over the table;
// here the table is rebuilt once with every column's new type and the
// rows put back through the USING expressions.
type alterColumnType struct {
	table   string
	columns []retypedColumn
}

type retypedColumn struct {
	name     string
	typeSQL  string // the PG type as written
	usingSQL string // the USING expression as written; "" for none
}

func parseAlterColumnType(tokens []Token) (*alterColumnType, bool) {
	words, pos := significant(tokens)
	is := func(i int, w string) bool {
		return i < len(words) && isNamePart(words[i]) && strings.EqualFold(words[i].Raw, w)
	}
	table, i, ok := alterTableWords(words)
	if !ok {
		return nil, false
	}
	d := &alterColumnType{table: table}
	for {
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
		col := retypedColumn{name: foldIdent(words[i])}
		i++
		if is(i, "SET") && is(i+1, "DATA") {
			i += 2
		}
		if !is(i, "TYPE") {
			return nil, false
		}
		i++
		// The action ends at a top-level comma; the type and the USING
		// expression keep their original text, so a cast such as
		// id::uuid reaches the translator as written.
		end, depth := i, 0
		for ; end < len(words); end++ {
			switch {
			case words[end].Kind == TokParen && words[end].Value == "(":
				depth++
			case words[end].Kind == TokParen && words[end].Value == ")":
				depth--
			case words[end].Kind == TokComma && depth == 0:
			}
			if words[end].Kind == TokComma && depth == 0 {
				break
			}
		}
		j := i
		for j < end && !is(j, "USING") {
			j++
		}
		if j == i {
			return nil, false
		}
		stop := len(tokens)
		if end < len(words) {
			stop = pos[end]
		}
		typeEnd := stop
		if j < end {
			if j+1 == end {
				return nil, false
			}
			typeEnd = pos[j]
			col.usingSQL = strings.TrimSpace(Reassemble(tokens[pos[j+1]:stop]))
		}
		col.typeSQL = strings.TrimSpace(Reassemble(tokens[pos[i]:typeEnd]))
		d.columns = append(d.columns, col)
		if end == len(words) {
			return d, true
		}
		i = end + 1
	}
}

func (d *alterColumnType) exec(c *conn) (driver.Result, error) {
	t, err := c.lookupTable(d.table)
	if err != nil {
		return nil, err
	}
	createSQL := t.sql
	exprs := map[string]string{}
	for _, col := range d.columns {
		var ok bool
		if createSQL, ok = retypeColumnSQL(createSQL, col); !ok {
			return nil, pgErrorf("42703", `column "%s" of relation "%s" does not exist`, col.name, d.table)
		}
		if col.usingSQL != "" {
			exprs[strings.ToLower(col.name)] = col.usingSQL
		}
	}
	copyBack, err := c.copyBackSQL(t, exprs)
	if err != nil {
		return nil, err
	}
	if err := c.rebuildTable(t, createSQL, copyBack); err != nil {
		return nil, err
	}
	return driver.ResultNoRows, nil
}

// columnConstraintKeywords end a column's type in a column definition.
var columnConstraintKeywords = map[string]bool{
	"CONSTRAINT": true, "PRIMARY": true, "NOT": true, "NULL": true, "UNIQUE": true,
	"CHECK": true, "DEFAULT": true, "REFERENCES": true, "COLLATE": true,
	"GENERATED": true, "AS": true,
}

// retypeColumnSQL rewrites a stored CREATE TABLE statement with col's type
// replaced by the translation of its typeSQL. It reports false when the
// column is not defined.
func retypeColumnSQL(tableSQL string, col retypedColumn) (string, bool) {
	tokens := Tokenize(tableSQL)
	for _, def := range tableDefinitions(tokens) {
		if def.constraint || !strings.EqualFold(unquoteIdent(tokens[def.start].Raw), col.name) {
			continue
		}
		name := tokens[def.start]
		start, end := columnTypeSpan(tokens, def.start+1)
		newType := translateTypes(append([]Token{
			{Kind: TokParen, Value: "(", Raw: "("}, name, {Kind: TokWhitespace, Value: " ", Raw: " "},
		}, Tokenize(col.typeSQL)...), true)[3:]
		out := append([]Token{}, tokens[:start]...)
		out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
		out = append(out, newType...)
		out = append(out, Token{Kind: TokWhitespace, Value: " ", Raw: " "})
		out = append(out, tokens[end:]...)
		return Reassemble(out), true
	}
	return "", false
}

// tableDef is one definition inside a CREATE TABLE's parentheses: a
// column, or a table constraint. start and end index tokens; comma is
// the index of the comma before it, or -1 for the first.
type tableDef struct {
	start, end int
	comma      int
	constraint bool
}

var tableConstraintKeywords = map[string]bool{
	"CONSTRAINT": true, "PRIMARY": true, "UNIQUE": true, "CHECK": true, "FOREIGN": true,
}

// tableDefinitions splits a CREATE TABLE's body into its definitions.
func tableDefinitions(tokens []Token) []tableDef {
	var defs []tableDef
	depth := 0
	cur := tableDef{start: -1, comma: -1}
	for i, t := range tokens {
		switch {
		case t.Kind == TokParen && t.Value == "(":
			depth++
			continue
		case t.Kind == TokParen && t.Value == ")":
			depth--
			if depth == 0 && cur.start >= 0 {
				cur.end = i
				defs = append(defs, cur)
				cur = tableDef{start: -1, comma: -1}
			}
			continue
		case t.Kind == TokWhitespace || t.Kind == TokComment:
			continue
		case t.Kind == TokComma && depth == 1:
			if cur.start >= 0 {
				cur.end = i
				defs = append(defs, cur)
			}
			cur = tableDef{start: -1, comma: i}
			continue
		}
		if depth == 1 && cur.start < 0 {
			cur.start = i
			cur.constraint = isNamePart(t) && tableConstraintKeywords[strings.ToUpper(t.Raw)]
		}
	}
	return defs
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

// ---- DROP CONSTRAINT --------------------------------------------------------

// dropConstraint is a parsed ALTER TABLE table DROP CONSTRAINT [IF EXISTS] name.
// The constraint is found by its name as written (CONSTRAINT name ...) or
// by the name the catalogs synthesise for a foreign key,
// <table>_<column>_fkey, and the table is rebuilt without it.
type dropConstraint struct {
	table    string
	name     string
	ifExists bool
}

func parseDropConstraint(tokens []Token) (*dropConstraint, bool) {
	words, _ := significant(tokens)
	is := func(i int, w string) bool {
		return i < len(words) && isNamePart(words[i]) && strings.EqualFold(words[i].Raw, w)
	}
	table, i, ok := alterTableWords(words)
	if !ok || !is(i, "DROP") || !is(i+1, "CONSTRAINT") {
		return nil, false
	}
	i += 2
	d := &dropConstraint{table: table}
	if is(i, "IF") && is(i+1, "EXISTS") {
		d.ifExists = true
		i += 2
	}
	if i >= len(words) || !isNamePart(words[i]) {
		return nil, false
	}
	d.name = foldIdent(words[i])
	i++
	if is(i, "RESTRICT") || is(i, "CASCADE") {
		i++
	}
	if i != len(words) {
		return nil, false
	}
	return d, true
}

func (d *dropConstraint) exec(c *conn) (driver.Result, error) {
	t, err := c.lookupTable(d.table)
	if err != nil {
		return nil, err
	}
	tokens := Tokenize(t.sql)
	start, end, ok := namedConstraintSpan(tokens, d.name)
	if !ok {
		cols, err := c.foreignKeyColumns(t, d.name)
		if err != nil {
			return nil, err
		}
		if cols != nil {
			start, end, ok = foreignKeySpan(tokens, cols)
		}
	}
	if !ok {
		if d.ifExists {
			return driver.ResultNoRows, nil
		}
		return nil, pgErrorf("42704", `constraint "%s" of relation "%s" does not exist`, d.name, d.table)
	}
	createSQL := Reassemble(append(append([]Token{}, tokens[:start]...), tokens[end:]...))
	copyBack, err := c.copyBackSQL(t, nil)
	if err != nil {
		return nil, err
	}
	if err := c.rebuildTable(t, createSQL, copyBack); err != nil {
		return nil, err
	}
	return driver.ResultNoRows, nil
}

// foreignKeyColumns resolves a synthesised foreign key name,
// <table>_<columns>_fkey, to the constraint's columns, or nil.
func (c *conn) foreignKeyColumns(t storedTable, name string) ([]string, error) {
	rows, err := c.queryDirectStrings(`SELECT id || ' ' || "from" FROM pragma_foreign_key_list(?) ORDER BY id, seq`, t.name)
	if err != nil {
		return nil, wrapError(err)
	}
	byID := map[string][]string{}
	var ids []string
	for _, r := range rows {
		id, col, _ := strings.Cut(r, " ")
		if _, seen := byID[id]; !seen {
			ids = append(ids, id)
		}
		byID[id] = append(byID[id], col)
	}
	rel := strings.ToLower(t.name)
	if i := strings.LastIndexByte(rel, '.'); i >= 0 {
		rel = rel[i+1:]
	}
	for _, id := range ids {
		if strings.EqualFold(foreignKeyName(rel, byID[id]), name) {
			return byID[id], nil
		}
	}
	return nil, nil
}

// foreignKeyName is the name PostgreSQL gives an unnamed foreign key.
func foreignKeyName(table string, cols []string) string {
	return table + "_" + strings.Join(cols, "_") + "_fkey"
}

// namedConstraintSpan finds CONSTRAINT name in a CREATE TABLE: a table
// constraint is removed whole, with the comma before it; a column
// constraint from CONSTRAINT to the end of its clause.
func namedConstraintSpan(tokens []Token, name string) (start, end int, ok bool) {
	for _, def := range tableDefinitions(tokens) {
		for i := def.start; i < def.end; i++ {
			t := tokens[i]
			if !isNamePart(t) || !strings.EqualFold(t.Raw, "CONSTRAINT") {
				continue
			}
			j := nextNonTrivial(tokens, i+1)
			if j == -1 || j >= def.end || !strings.EqualFold(unquoteIdent(tokens[j].Raw), name) {
				continue
			}
			if def.constraint {
				return defSpan(def), def.end, true
			}
			return i, constraintClauseEnd(tokens, j+1, def.end), true
		}
	}
	return 0, 0, false
}

// foreignKeySpan finds the foreign key on cols: a table-level FOREIGN KEY
// (cols) definition, or for one column the REFERENCES clause in its
// column definition.
func foreignKeySpan(tokens []Token, cols []string) (start, end int, ok bool) {
	for _, def := range tableDefinitions(tokens) {
		if def.constraint {
			i := def.start
			if strings.EqualFold(tokens[i].Raw, "CONSTRAINT") {
				i = nextNonTrivial(tokens, nextNonTrivial(tokens, i+1)+1)
			}
			if i == -1 || !strings.EqualFold(tokens[i].Raw, "FOREIGN") {
				continue
			}
			p := nextNonTrivial(tokens, nextNonTrivial(tokens, i+1)+1) // after KEY
			if p != -1 && tokens[p].Kind == TokParen && sameColumns(parenNames(tokens, p), cols) {
				return defSpan(def), def.end, true
			}
			continue
		}
		if len(cols) != 1 || !strings.EqualFold(unquoteIdent(tokens[def.start].Raw), cols[0]) {
			continue
		}
		for i := def.start + 1; i < def.end; i++ {
			if isNamePart(tokens[i]) && strings.EqualFold(tokens[i].Raw, "REFERENCES") {
				return i, constraintClauseEnd(tokens, i+1, def.end), true
			}
		}
	}
	return 0, 0, false
}

// defSpan is where a table definition's removal starts: at the comma
// before it, or at the definition itself for the first one.
func defSpan(def tableDef) int {
	if def.comma >= 0 {
		return def.comma
	}
	return def.start
}

// constraintClauseEnd is where a column constraint's clause ends: at the
// next column constraint keyword at the definition's depth, or the
// definition's end. NOT NULL counts; the NOT of NOT DEFERRABLE does not.
func constraintClauseEnd(tokens []Token, from, defEnd int) int {
	depth := 0
	for i := from; i < defEnd; i++ {
		t := tokens[i]
		switch {
		case t.Kind == TokParen && t.Value == "(":
			depth++
		case t.Kind == TokParen && t.Value == ")":
			depth--
		case depth == 0 && isNamePart(t):
			kw := strings.ToUpper(t.Raw)
			if kw == "NOT" {
				j := nextNonTrivial(tokens, i+1)
				if j != -1 && strings.EqualFold(tokens[j].Raw, "NULL") {
					return i
				}
				continue
			}
			if kw == "NULL" {
				continue
			}
			if columnConstraintKeywords[kw] {
				return i
			}
		}
	}
	return defEnd
}

// parenNames reads the names inside a parenthesised list at tokens[p].
func parenNames(tokens []Token, p int) []string {
	var names []string
	for i := p + 1; i < len(tokens); i++ {
		t := tokens[i]
		if t.Kind == TokParen && t.Value == ")" {
			break
		}
		if isNamePart(t) {
			names = append(names, unquoteIdent(t.Raw))
		}
	}
	return names
}

func sameColumns(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}

// ---- ADD CONSTRAINT ... FOREIGN KEY -----------------------------------------

// addForeignKey is a parsed
//
//	ALTER TABLE table ADD CONSTRAINT name FOREIGN KEY (cols) REFERENCES parent (cols) [actions]
//
// The constraint is appended to the table's definitions, as written, and
// the table rebuilt; existing rows that violate it fail the rebuild.
type addForeignKey struct {
	table  string
	clause string // CONSTRAINT name FOREIGN KEY ... as written
}

func parseAddForeignKey(tokens []Token) (*addForeignKey, bool) {
	words, pos := significant(tokens)
	is := func(i int, w string) bool {
		return i < len(words) && isNamePart(words[i]) && strings.EqualFold(words[i].Raw, w)
	}
	table, i, ok := alterTableWords(words)
	if !ok || !is(i, "ADD") || !is(i+1, "CONSTRAINT") {
		return nil, false
	}
	if i+2 >= len(words) || !isNamePart(words[i+2]) || !is(i+3, "FOREIGN") || !is(i+4, "KEY") {
		return nil, false
	}
	clause := strings.TrimSpace(Reassemble(translateSchemas(tokens[pos[i+1]:])))
	return &addForeignKey{table: table, clause: clause}, true
}

func (d *addForeignKey) exec(c *conn) (driver.Result, error) {
	t, err := c.lookupTable(d.table)
	if err != nil {
		return nil, err
	}
	tokens := Tokenize(t.sql)
	defs := tableDefinitions(tokens)
	if len(defs) == 0 {
		return nil, pgErrorf("42P01", `relation "%s" does not exist`, d.table)
	}
	closing := defs[len(defs)-1].end
	for closing < len(tokens) && (tokens[closing].Kind != TokParen || tokens[closing].Value != ")") {
		closing++
	}
	createSQL := Reassemble(tokens[:closing]) + ", " + d.clause + Reassemble(tokens[closing:])
	copyBack, err := c.copyBackSQL(t, nil)
	if err != nil {
		return nil, err
	}
	if err := c.rebuildTable(t, createSQL, copyBack); err != nil {
		return nil, err
	}
	return driver.ResultNoRows, nil
}
