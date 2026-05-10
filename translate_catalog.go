package pglike

import "strings"

// catalogSchemaMap maps qualified PG catalog references to the mangled
// SQLite view names installed by installCatalogViews.
//
//	information_schema.<view> → _pglike_information_schema_<view>
//	pg_catalog.<view>         → _pglike_<view>   (only ones we actually expose)
var catalogSchemaMap = map[string]map[string]string{
	"information_schema": {
		"tables":                  "_pglike_information_schema_tables",
		"columns":                 "_pglike_information_schema_columns",
		"table_constraints":       "_pglike_information_schema_table_constraints",
		"key_column_usage":        "_pglike_information_schema_key_column_usage",
		"referential_constraints": "_pglike_information_schema_referential_constraints",
		"constraint_column_usage": "_pglike_information_schema_constraint_column_usage",
	},
	"pg_catalog": {
		"pg_indexes":       "_pglike_pg_indexes",
		"pg_index_columns": "_pglike_pg_index_columns",
	},
}

// catalogBareMap maps unqualified PG catalog references that PG users
// typically write without the pg_catalog. prefix.
var catalogBareMap = map[string]string{
	"pg_indexes":       "_pglike_pg_indexes",
	"pg_index_columns": "_pglike_pg_index_columns",
}

// translateCatalog rewrites PG catalog references to the mangled view names
// created by installCatalogViews. SQLite resolves `a.b` as `<attached-db>.<table>`,
// so `information_schema.columns` won't work directly — the views live in
// the main schema with collapsed names instead. The same pglike-using SQL
// runs unchanged against real Postgres, where `information_schema.*` is real.
func translateCatalog(tokens []Token) []Token {
	out := make([]Token, 0, len(tokens))
	prevNonTrivial := -1 // index into `out`, or -1
	skipUntil := -1

	for i := 0; i < len(tokens); i++ {
		if i <= skipUntil {
			continue
		}
		t := tokens[i]

		if t.Kind != TokIdent {
			out = append(out, t)
			if t.Kind != TokWhitespace && t.Kind != TokComment {
				prevNonTrivial = len(out) - 1
			}
			continue
		}

		// If the prior non-trivial token is a Dot, this ident is the RHS of
		// some other qualifier (e.g. user.pg_indexes); leave it alone.
		prevIsDot := prevNonTrivial >= 0 && out[prevNonTrivial].Kind == TokDot

		val := strings.ToLower(t.Value)

		if !prevIsDot {
			// Try schema-qualified rewrite: <schema> . <view>
			if mp, ok := catalogSchemaMap[val]; ok {
				j := nextNonTrivial(tokens, i+1)
				if j != -1 && tokens[j].Kind == TokDot {
					k := nextNonTrivial(tokens, j+1)
					if k != -1 && tokens[k].Kind == TokIdent {
						if mapped, ok := mp[strings.ToLower(tokens[k].Value)]; ok {
							out = append(out, Token{Kind: TokIdent, Value: mapped, Raw: mapped})
							prevNonTrivial = len(out) - 1
							skipUntil = k
							continue
						}
					}
				}
			}
			// Try bare rewrite: pg_indexes
			if mapped, ok := catalogBareMap[val]; ok {
				out = append(out, Token{Kind: TokIdent, Value: mapped, Raw: mapped})
				prevNonTrivial = len(out) - 1
				continue
			}
		}

		out = append(out, t)
		prevNonTrivial = len(out) - 1
	}
	return out
}

// nextNonTrivial returns the index of the next token that is not whitespace
// or a comment, starting at i. Returns -1 if none.
func nextNonTrivial(tokens []Token, i int) int {
	for ; i < len(tokens); i++ {
		if tokens[i].Kind != TokWhitespace && tokens[i].Kind != TokComment {
			return i
		}
	}
	return -1
}
