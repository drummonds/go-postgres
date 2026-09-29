package pglike

import (
	"sort"
	"strings"
)

// translateSchemas rewrites schema-qualified names into the single SQLite
// table that stores them: crm.customers → "crm.customers", public.t → t.
// A dot can only appear inside a PG identifier when quoted, so the stored
// name can't collide with an ordinary table.
//
// Rewriting is syntactic, not driven by which schemas exist, so translation
// stays a pure function of the SQL:
//   - two-part names only where a relation is expected (after FROM, JOIN,
//     INTO, UPDATE, TABLE, VIEW, INDEX, REFERENCES, and ON in CREATE INDEX),
//     so alias.col is never touched;
//   - SELECT/DML sources get an implicit alias (AS customers) when none is
//     given, keeping PG's correlation name so customers.id still resolves;
//   - three-part schema.rel.col drops the schema, resolving via that alias;
//   - CREATE INDEX name ON crm.t names the index "crm.name", because PG
//     places an index in its table's schema but SQLite index names are global.
func translateSchemas(tokens []Token) []Token {
	var edits []tokenEdit
	if e, ok := createIndexNameEdit(tokens); ok {
		edits = append(edits, e)
	}
	createIndex := len(edits) > 0 || isCreateIndex(tokens)

	var subquery []bool        // paren stack: does this paren open a subquery?
	fromList := map[int]bool{} // depth → inside a FROM list at that depth
	consumed := -1             // last token index rewritten as a relation
	prev := -1                 // previous non-trivial token index
	inSelectScope := func() bool { return len(subquery) == 0 || subquery[len(subquery)-1] }
	tryRel := func(from int, alias bool) {
		if e, end, ok := relationEdit(tokens, from, alias); ok {
			edits = append(edits, e)
			consumed = end
		}
	}

	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t.Kind == TokWhitespace || t.Kind == TokComment {
			continue
		}
		depth := len(subquery)
		switch {
		case t.Kind == TokParen && t.Value == "(":
			j := nextNonTrivial(tokens, i+1)
			subquery = append(subquery, j != -1 && tokens[j].Kind == TokKeyword &&
				(tokens[j].Value == "SELECT" || tokens[j].Value == "WITH" || tokens[j].Value == "VALUES"))
		case t.Kind == TokParen && t.Value == ")":
			delete(fromList, depth)
			if depth > 0 {
				subquery = subquery[:depth-1]
			}
		case t.Kind == TokComma:
			if fromList[depth] {
				tryRel(i+1, true)
			}
		case t.Kind == TokKeyword:
			switch t.Value {
			case "FROM":
				isDistinctFrom := prev >= 0 && tokens[prev].Kind == TokKeyword && tokens[prev].Value == "DISTINCT"
				if inSelectScope() && !isDistinctFrom {
					tryRel(i+1, true)
					fromList[depth] = true
				}
			case "JOIN", "INTO", "UPDATE":
				tryRel(i+1, true)
			case "TABLE", "VIEW", "INDEX", "REFERENCES", "TRUNCATE":
				tryRel(i+1, false)
			case "ON":
				delete(fromList, depth)
				if createIndex {
					tryRel(i+1, false)
				}
			case "WHERE", "GROUP", "ORDER", "LIMIT", "HAVING", "UNION", "EXCEPT",
				"INTERSECT", "USING", "RETURNING", "WINDOW", "OFFSET", "SET":
				delete(fromList, depth)
			}
		}
		// Three-part schema.rel.col outside a rewritten relation: drop "schema.".
		if i > consumed && isNamePart(t) && (prev < 0 || tokens[prev].Kind != TokDot) {
			if d1 := nextNonTrivial(tokens, i+1); d1 != -1 && tokens[d1].Kind == TokDot {
				if r := nextNonTrivial(tokens, d1+1); r != -1 && isNamePart(tokens[r]) {
					if d2 := nextNonTrivial(tokens, r+1); d2 != -1 && tokens[d2].Kind == TokDot {
						edits = append(edits, tokenEdit{start: i, end: d1})
						consumed = d2
					}
				}
			}
		}
		prev = i
	}
	return applyEdits(tokens, edits)
}

// tokenEdit replaces tokens[start..end] (inclusive) with repl.
type tokenEdit struct {
	start, end int
	repl       []Token
}

func applyEdits(tokens []Token, edits []tokenEdit) []Token {
	if len(edits) == 0 {
		return tokens
	}
	sort.Slice(edits, func(a, b int) bool { return edits[a].start < edits[b].start })
	out := make([]Token, 0, len(tokens))
	i := 0
	for _, e := range edits {
		if e.start < i {
			continue // overlapping; first edit wins
		}
		out = append(out, tokens[i:e.start]...)
		out = append(out, e.repl...)
		i = e.end + 1
	}
	return append(out, tokens[i:]...)
}

// relationEdit matches [IF [NOT] EXISTS] [ONLY] schema.rel starting at
// from. alias adds an implicit AS <rel> unless an alias follows. end is the
// index of the rel token.
func relationEdit(tokens []Token, from int, alias bool) (tokenEdit, int, bool) {
	s := skipRelationPrefix(tokens, from)
	schema, rel, ok := qualifiedName(tokens, s)
	if !ok {
		return tokenEdit{}, 0, false
	}
	next := nextNonTrivial(tokens, rel+1)
	if foldIdent(tokens[schema]) == "public" {
		return tokenEdit{start: schema, end: rel, repl: []Token{tokens[rel]}}, rel, true
	}
	repl := []Token{quotedIdent(foldIdent(tokens[schema]) + "." + foldIdent(tokens[rel]))}
	hasAlias := next != -1 && (tokens[next].Kind == TokIdent || (tokens[next].Kind == TokKeyword && tokens[next].Value == "AS"))
	if alias && !hasAlias {
		a := tokens[rel]
		if !isQuotedIdent(a.Raw) {
			a = Token{Kind: TokIdent, Value: foldIdent(a), Raw: foldIdent(a)}
		}
		repl = append(repl,
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
			Token{Kind: TokKeyword, Value: "AS", Raw: "AS"},
			Token{Kind: TokWhitespace, Value: " ", Raw: " "},
			a)
	}
	return tokenEdit{start: schema, end: rel, repl: repl}, rel, true
}

// qualifiedName matches exactly two dotted name parts at s, returning the
// token indexes of each part.
func qualifiedName(tokens []Token, s int) (schema, rel int, ok bool) {
	if s == -1 || !isNamePart(tokens[s]) {
		return 0, 0, false
	}
	d := nextNonTrivial(tokens, s+1)
	if d == -1 || tokens[d].Kind != TokDot {
		return 0, 0, false
	}
	r := nextNonTrivial(tokens, d+1)
	if r == -1 || !isNamePart(tokens[r]) {
		return 0, 0, false
	}
	if n := nextNonTrivial(tokens, r+1); n != -1 && tokens[n].Kind == TokDot {
		return 0, 0, false
	}
	return s, r, true
}

// skipRelationPrefix skips IF [NOT] EXISTS and ONLY before a relation name.
func skipRelationPrefix(tokens []Token, from int) int {
	j := nextNonTrivial(tokens, from)
	for j != -1 && tokens[j].Kind == TokKeyword {
		switch tokens[j].Value {
		case "IF", "NOT", "EXISTS", "ONLY":
			j = nextNonTrivial(tokens, j+1)
			continue
		}
		break
	}
	return j
}

// isCreateIndex reports whether the statement is CREATE [UNIQUE] INDEX.
func isCreateIndex(tokens []Token) bool {
	_, ok := createIndexName(tokens)
	return ok
}

// createIndexName returns the index of the name token in
// CREATE [UNIQUE] INDEX [IF NOT EXISTS] name ON ...
func createIndexName(tokens []Token) (int, bool) {
	j := nextNonTrivial(tokens, 0)
	if j == -1 || tokens[j].Kind != TokKeyword || tokens[j].Value != "CREATE" {
		return 0, false
	}
	j = nextNonTrivial(tokens, j+1)
	if j != -1 && tokens[j].Kind == TokKeyword && tokens[j].Value == "UNIQUE" {
		j = nextNonTrivial(tokens, j+1)
	}
	if j == -1 || tokens[j].Kind != TokKeyword || tokens[j].Value != "INDEX" {
		return 0, false
	}
	n := skipRelationPrefix(tokens, j+1)
	if n == -1 || !isNamePart(tokens[n]) {
		return 0, false
	}
	return n, true
}

// createIndexNameEdit moves the index name into the schema of its table.
func createIndexNameEdit(tokens []Token) (tokenEdit, bool) {
	n, ok := createIndexName(tokens)
	if !ok {
		return tokenEdit{}, false
	}
	on := nextNonTrivial(tokens, n+1)
	if on == -1 || tokens[on].Kind != TokKeyword || tokens[on].Value != "ON" {
		return tokenEdit{}, false
	}
	schema, _, ok := qualifiedName(tokens, skipRelationPrefix(tokens, on+1))
	if !ok || foldIdent(tokens[schema]) == "public" {
		return tokenEdit{}, false
	}
	name := quotedIdent(foldIdent(tokens[schema]) + "." + foldIdent(tokens[n]))
	return tokenEdit{start: n, end: n, repl: []Token{name}}, true
}

func isNamePart(t Token) bool {
	return t.Kind == TokIdent || t.Kind == TokKeyword
}

// foldIdent returns an identifier as PG resolves it: quoted names verbatim,
// unquoted names lower-cased.
func foldIdent(t Token) string {
	if isQuotedIdent(t.Raw) {
		return unquoteIdent(t.Raw)
	}
	return strings.ToLower(t.Raw)
}

func isQuotedIdent(s string) bool {
	return len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"'
}

func quotedIdent(name string) Token {
	q := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	return Token{Kind: TokIdent, Value: q, Raw: q}
}
