package pglike

import (
	"strings"

	"github.com/ncruces/go-sqlite3"
)

// pgDataTypes maps PG udt_name to information_schema.columns.data_type.
var pgDataTypes = map[string]string{
	"int2":        "smallint",
	"int4":        "integer",
	"int8":        "bigint",
	"float4":      "real",
	"float8":      "double precision",
	"bool":        "boolean",
	"varchar":     "character varying",
	"bpchar":      "character",
	"timestamp":   "timestamp without time zone",
	"timestamptz": "timestamp with time zone",
	"time":        "time without time zone",
	"timetz":      "time with time zone",
}

// sqliteDeclUDT maps declared types the translator leaves alone (they are
// valid in both dialects) to PG's udt_name.
var sqliteDeclUDT = map[string]string{
	"INTEGER": "int4", "INT": "int4", "TEXT": "text", "REAL": "float4", "BLOB": "bytea",
}

// pgColumnUDT returns the PG udt_name of column in the table whose
// sqlite_master.sql is tableSQL. It prefers the /*pg:<udt>*/ annotation the
// DDL translator leaves after a rewritten type, and otherwise derives it from
// SQLite's declared type (tables created before annotations, or with types
// the translator doesn't rewrite).
func pgColumnUDT(tableSQL, column, declType string) string {
	if udt := annotatedUDT(tableSQL, column); udt != "" {
		return udt
	}
	base := strings.ToUpper(strings.TrimSpace(declType))
	if i := strings.IndexByte(base, '('); i >= 0 {
		base = strings.TrimSpace(base[:i])
	}
	if udt, ok := sqliteDeclUDT[base]; ok {
		return udt
	}
	if udt, ok := pgUDT[base]; ok {
		return udt
	}
	return strings.ToLower(declType)
}

// pgColumnDataType returns information_schema.columns.data_type for a udt_name.
func pgColumnDataType(udt string) string {
	if dt, ok := pgDataTypes[udt]; ok {
		return dt
	}
	return udt
}

// annotatedUDT finds column's definition in a CREATE TABLE statement and
// returns the udt from its /*pg:...*/ comment, or "" if there is none.
func annotatedUDT(tableSQL, column string) string {
	tokens := Tokenize(tableSQL)
	depth := 0
	atDefStart := false
	inColumn := false
	for _, t := range tokens {
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
			if inColumn {
				return ""
			}
			atDefStart = true
			continue
		case t.Kind == TokWhitespace:
			continue
		}
		if depth != 1 {
			continue
		}
		if atDefStart {
			atDefStart = false
			inColumn = strings.EqualFold(unquoteIdent(t.Raw), column)
			continue
		}
		if inColumn && t.Kind == TokComment && strings.HasPrefix(t.Raw, pgTypeCommentPrefix) {
			return strings.TrimSuffix(strings.TrimPrefix(t.Raw, pgTypeCommentPrefix), "*/")
		}
	}
	return ""
}

// unquoteIdent strips SQL double quotes from an identifier.
func unquoteIdent(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return strings.ReplaceAll(s[1:len(s)-1], `""`, `"`)
	}
	return s
}

// registerCatalogFunctions registers the helpers the catalog views use to
// report PG column types.
func registerCatalogFunctions(conn *sqlite3.Conn) error {
	udt := func(arg []sqlite3.Value) string {
		return pgColumnUDT(arg[0].Text(), arg[1].Text(), arg[2].Text())
	}
	err := conn.CreateFunction("_pglike_udt_name", 3, sqlite3.DETERMINISTIC,
		func(ctx sqlite3.Context, arg ...sqlite3.Value) {
			ctx.ResultText(udt(arg))
		})
	if err != nil {
		return err
	}
	return conn.CreateFunction("_pglike_data_type", 3, sqlite3.DETERMINISTIC,
		func(ctx sqlite3.Context, arg ...sqlite3.Value) {
			ctx.ResultText(pgColumnDataType(udt(arg)))
		})
}
