package pglike

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
)

// schemasTable records schemas created with CREATE SCHEMA. public is
// implicit and never stored. The _pglike_ prefix keeps it out of the catalogs.
const schemasTable = "_pglike_schemas"

// schemaDDL is a parsed CREATE SCHEMA or DROP SCHEMA. Both run in Go rather
// than as translated SQL: DROP SCHEMA ... CASCADE must drop a variable set
// of tables and views, and both need PG's SQLSTATEs.
type schemaDDL struct {
	create   bool
	names    []string
	ifClause bool // IF NOT EXISTS (create) / IF EXISTS (drop)
	cascade  bool
}

// parseSchemaDDL recognises
//
//	CREATE SCHEMA [IF NOT EXISTS] name [AUTHORIZATION role]
//	DROP SCHEMA [IF EXISTS] name [, ...] [CASCADE | RESTRICT]
func parseSchemaDDL(tokens []Token) (*schemaDDL, bool) {
	var words []Token
	for _, t := range tokens {
		if t.Kind != TokWhitespace && t.Kind != TokComment {
			words = append(words, t)
		}
	}
	is := func(i int, w string) bool {
		return i < len(words) && isNamePart(words[i]) && strings.EqualFold(words[i].Raw, w)
	}
	if len(words) < 3 || !is(1, "SCHEMA") {
		return nil, false
	}
	d := &schemaDDL{}
	i := 2
	switch {
	case is(0, "CREATE"):
		d.create = true
		if is(i, "IF") && is(i+1, "NOT") && is(i+2, "EXISTS") {
			d.ifClause = true
			i += 3
		}
	case is(0, "DROP"):
		if is(i, "IF") && is(i+1, "EXISTS") {
			d.ifClause = true
			i += 2
		}
	default:
		return nil, false
	}
	for i < len(words) && isNamePart(words[i]) {
		d.names = append(d.names, foldIdent(words[i]))
		i++
		if d.create || i >= len(words) || words[i].Kind != TokComma {
			break
		}
		i++
	}
	if len(d.names) == 0 {
		return nil, false
	}
	switch {
	case i == len(words):
	case d.create && is(i, "AUTHORIZATION") && i+2 == len(words):
	case !d.create && (is(i, "CASCADE") || is(i, "RESTRICT")) && i+1 == len(words):
		d.cascade = is(i, "CASCADE")
	default:
		return nil, false
	}
	return d, true
}

func pgErrorf(code, format string, args ...any) error {
	return &PGError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// execSchemaDDL runs a CREATE or DROP SCHEMA on this connection.
func (c *conn) execSchemaDDL(d *schemaDDL) (driver.Result, error) {
	for _, name := range d.names {
		var err error
		if d.create {
			err = c.createSchema(name, d.ifClause)
		} else {
			err = c.dropSchema(name, d.ifClause, d.cascade)
		}
		if err != nil {
			return nil, err
		}
	}
	return driver.ResultNoRows, nil
}

func (c *conn) schemaExists(name string) (bool, error) {
	if name == "public" {
		return true, nil
	}
	rows, err := c.queryDirectStrings(`SELECT schema_name FROM `+schemasTable+` WHERE schema_name = ?`, name)
	return len(rows) > 0, err
}

func (c *conn) createSchema(name string, ifNotExists bool) error {
	exists, err := c.schemaExists(name)
	if err != nil {
		return wrapError(err)
	}
	if exists {
		if ifNotExists {
			return nil
		}
		return pgErrorf("42P06", "schema %q already exists", name)
	}
	return wrapError(c.execDirectArgs(`INSERT INTO `+schemasTable+` (schema_name) VALUES (?)`, name))
}

func (c *conn) dropSchema(name string, ifExists, cascade bool) error {
	if name == "public" {
		return pgErrorf("0A000", "pglike: dropping schema public is not supported")
	}
	exists, err := c.schemaExists(name)
	if err != nil {
		return wrapError(err)
	}
	if !exists {
		if ifExists {
			return nil
		}
		return pgErrorf("3F000", "schema %q does not exist", name)
	}
	// Views first, so none is left referencing a dropped table.
	objects, err := c.queryDirectStrings(`SELECT type || ' ' || name FROM sqlite_master
		WHERE type IN ('table', 'view') AND substr(name, 1, length(?1) + 1) = ?1 || '.'
		ORDER BY type = 'table', name`, name)
	if err != nil {
		return wrapError(err)
	}
	if len(objects) > 0 && !cascade {
		return pgErrorf("2BP01", "cannot drop schema %s because other objects depend on it", name)
	}
	// One savepoint so a failed drop leaves the schema intact. Deferring FK
	// checks lets tables that reference each other be dropped in any order.
	stmts := []string{"SAVEPOINT pglike_drop_schema", "PRAGMA defer_foreign_keys = ON"}
	for _, o := range objects {
		kind, obj, _ := strings.Cut(o, " ")
		stmts = append(stmts, "DROP "+strings.ToUpper(kind)+" "+quotedIdent(obj).Raw)
	}
	for _, s := range stmts {
		if err := c.execDirect(s); err != nil {
			_ = c.execDirect("ROLLBACK TO pglike_drop_schema")
			_ = c.execDirect("RELEASE pglike_drop_schema")
			return wrapError(err)
		}
	}
	if err := c.execDirectArgs(`DELETE FROM `+schemasTable+` WHERE schema_name = ?`, name); err != nil {
		_ = c.execDirect("ROLLBACK TO pglike_drop_schema")
		_ = c.execDirect("RELEASE pglike_drop_schema")
		return wrapError(err)
	}
	return wrapError(c.execDirect("RELEASE pglike_drop_schema"))
}

// execDirectArgs executes untranslated SQL with bind arguments.
func (c *conn) execDirectArgs(sql string, args ...driver.Value) error {
	s, err := c.inner.Prepare(sql)
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = s.Exec(args) //nolint:staticcheck
	return err
}

// queryDirectStrings runs untranslated SQL and returns the first column of
// each row as a string.
func (c *conn) queryDirectStrings(sql string, args ...driver.Value) ([]string, error) {
	s, err := c.inner.Prepare(sql)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	r, err := s.Query(args) //nolint:staticcheck
	if err != nil {
		return nil, err
	}
	defer r.Close()
	dest := make([]driver.Value, len(r.Columns()))
	var out []string
	for {
		err := r.Next(dest)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprint(dest[0]))
	}
}

// schemaStmt is a prepared CREATE/DROP SCHEMA.
type schemaStmt struct {
	c   *conn
	ddl *schemaDDL
}

func (s *schemaStmt) Close() error  { return nil }
func (s *schemaStmt) NumInput() int { return 0 }
func (s *schemaStmt) Exec([]driver.Value) (driver.Result, error) {
	return s.c.execSchemaDDL(s.ddl)
}
func (s *schemaStmt) Query([]driver.Value) (driver.Rows, error) {
	return nil, errors.New("pglike: CREATE/DROP SCHEMA returns no rows")
}
