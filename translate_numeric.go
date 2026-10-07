package pglike

import (
	"strconv"
	"strings"
)

// translateNumeric rewrites every expression that has a ::numeric or
// CAST(... AS NUMERIC) operand into pg_numeric_* calls (see numeric.go),
// so SQLite evaluates it exactly and with PG's scale rules rather than as
// integer division, REAL arithmetic or a text comparison. The numeric
// property propagates through + - * /, unary minus, parentheses, round()
// and argument-passing calls such as coalesce(); a comparison with a
// numeric operand becomes pg_numeric_cmp(a, b) <op> 0.
//
// Anything without such an operand is left byte for byte as it was. Bare
// NUMERIC columns and decimal literals are not treated as numeric: a
// token-level translator cannot know a column's type, and treating every
// literal as numeric would retype ordinary REAL arithmetic.
//
// Runs before translateDDL, whose type pass would otherwise turn the
// NUMERIC keyword into TEXT.
func translateNumeric(tokens []Token) []Token {
	var out []Token
	for i := 0; i < len(tokens); {
		if numericStart(tokens[i], out) {
			p := numParser{toks: tokens, pos: i}
			if node, ok := p.parseExpr(); ok && node.rewritten {
				out = append(out, Token{Kind: TokIdent, Value: node.text, Raw: node.text})
				i = p.pos
				continue
			}
		}
		out = append(out, tokens[i])
		i++
	}
	return out
}

// numericStart reports whether an expression may begin at t. A sign is a
// start only when what precedes it cannot end an operand, so a binary
// minus after a span the parser declined is never taken for a unary one.
func numericStart(t Token, out []Token) bool {
	switch t.Kind {
	case TokNumber, TokString, TokParam, TokIdent:
		return true
	case TokParen:
		return t.Value == "("
	case TokKeyword:
		switch t.Value {
		case "NULL", "TRUE", "FALSE", "CAST", "COALESCE", "NULLIF":
			return true
		}
		return false
	case TokOperator:
		if t.Value != "-" && t.Value != "+" {
			return false
		}
		for j := len(out) - 1; j >= 0; j-- {
			switch out[j].Kind {
			case TokWhitespace, TokComment:
				continue
			case TokIdent, TokNumber, TokString, TokParam:
				return false
			case TokParen:
				return out[j].Value == "("
			case TokKeyword:
				switch out[j].Value {
				case "NULL", "TRUE", "FALSE", "END":
					return false
				}
				return true
			}
			return true
		}
		return true
	}
	return false
}

type numNode struct {
	numeric   bool   // evaluates to a PG numeric
	rewritten bool   // text differs from the source span
	text      string // the expression as it should be emitted
}

type numParser struct {
	toks []Token
	pos  int // next unconsumed token; trailing whitespace is never consumed
}

var numericFn = map[string]string{
	"+": "pg_numeric_add", "-": "pg_numeric_sub", "*": "pg_numeric_mul", "/": "pg_numeric_div",
}

func (p *numParser) peek() (int, bool) {
	for i := p.pos; i < len(p.toks); i++ {
		if k := p.toks[i].Kind; k != TokWhitespace && k != TokComment {
			return i, true
		}
	}
	return len(p.toks), false
}

func (p *numParser) peekOp(values ...string) (string, bool) {
	i, ok := p.peek()
	if !ok || p.toks[i].Kind != TokOperator {
		return "", false
	}
	for _, v := range values {
		if p.toks[i].Value == v {
			return v, true
		}
	}
	return "", false
}

func (p *numParser) peekIs(kind TokenKind, value string) bool {
	i, ok := p.peek()
	return ok && p.toks[i].Kind == kind && p.toks[i].Value == value
}

func (p *numParser) take() Token {
	i, _ := p.peek()
	p.pos = i + 1
	return p.toks[i]
}

func (p *numParser) span(start int) string {
	return strings.TrimSpace(Reassemble(p.toks[start:p.pos]))
}

// compose picks the emitted text: the rewritten form when any part
// changed, else the source span so untouched SQL keeps its formatting.
func (p *numParser) compose(start int, rewritten bool, text string) string {
	if rewritten {
		return text
	}
	return p.span(start)
}

// parseExpr: comparison over additive expressions.
func (p *numParser) parseExpr() (numNode, bool) {
	start := p.pos
	left, ok := p.parseAdditive()
	if !ok {
		return numNode{}, false
	}
	op, ok := p.peekOp("=", "<>", "!=", "<", ">", "<=", ">=")
	if !ok {
		return left, true
	}
	save := p.pos
	p.take()
	right, ok := p.parseAdditive()
	if !ok {
		p.pos = save
		return left, true
	}
	if left.numeric || right.numeric {
		return numNode{rewritten: true, text: "pg_numeric_cmp(" + left.text + ", " + right.text + ") " + op + " 0"}, true
	}
	rewritten := left.rewritten || right.rewritten
	return numNode{rewritten: rewritten, text: p.compose(start, rewritten, left.text+" "+op+" "+right.text)}, true
}

func (p *numParser) parseAdditive() (numNode, bool) {
	return p.parseBinary(p.parseMultiplicative, "+", "-")
}

func (p *numParser) parseMultiplicative() (numNode, bool) {
	return p.parseBinary(p.parseUnary, "*", "/")
}

func (p *numParser) parseBinary(operand func() (numNode, bool), ops ...string) (numNode, bool) {
	start := p.pos
	left, ok := operand()
	if !ok {
		return numNode{}, false
	}
	for {
		op, ok := p.peekOp(ops...)
		if !ok {
			return left, true
		}
		save := p.pos
		p.take()
		right, ok := operand()
		if !ok {
			p.pos = save
			return left, true
		}
		if left.numeric || right.numeric {
			left = numNode{numeric: true, rewritten: true, text: numericFn[op] + "(" + left.text + ", " + right.text + ")"}
			continue
		}
		rewritten := left.rewritten || right.rewritten
		left = numNode{rewritten: rewritten, text: p.compose(start, rewritten, left.text+" "+op+" "+right.text)}
	}
}

func (p *numParser) parseUnary() (numNode, bool) {
	start := p.pos
	op, ok := p.peekOp("-", "+")
	if !ok {
		return p.parsePostfix()
	}
	p.take()
	operand, ok := p.parseUnary()
	if !ok {
		return numNode{}, false
	}
	if operand.numeric {
		if op == "-" {
			return numNode{numeric: true, rewritten: true, text: "pg_numeric_neg(" + operand.text + ")"}, true
		}
		return operand, true
	}
	return numNode{rewritten: operand.rewritten, text: p.compose(start, operand.rewritten, op+operand.text)}, true
}

// parsePostfix: a primary followed by any number of ::type casts.
func (p *numParser) parsePostfix() (numNode, bool) {
	start := p.pos
	node, ok := p.parsePrimary()
	if !ok {
		return numNode{}, false
	}
	for {
		if _, ok := p.peekOp("::"); !ok {
			return node, true
		}
		save := p.pos
		p.take()
		i, ok := p.peek()
		if !ok || (p.toks[i].Kind != TokKeyword && p.toks[i].Kind != TokIdent) {
			p.pos = save
			return node, true
		}
		typeToks, end := extractTypeName(p.toks, i)
		p.pos = end + 1
		node = p.cast(start, node, assembleTypeName(typeToks), p.toks[i:end+1])
	}
}

// cast applies a cast to node. NUMERIC makes it numeric: pg_numeric(x), or
// pg_numeric_round(x, s) when the type carries a scale. Any other type on
// an already rewritten node is spelt out as CAST(... AS ...), since the
// later cast pass only sees whole tokens.
func (p *numParser) cast(start int, node numNode, typeName string, typeToks []Token) numNode {
	if typeName == "NUMERIC" || typeName == "DECIMAL" {
		if scale, ok := declaredScale(typeToks); ok {
			return numNode{numeric: true, rewritten: true, text: "pg_numeric_round(" + node.text + ", " + strconv.Itoa(scale) + ")"}
		}
		return numNode{numeric: true, rewritten: true, text: "pg_numeric(" + node.text + ")"}
	}
	if node.rewritten {
		return numNode{rewritten: true, text: "CAST(" + node.text + " AS " + mapCastType(typeName) + ")"}
	}
	return numNode{text: p.span(start)}
}

// declaredScale reads s from NUMERIC(p, s); NUMERIC(p) has scale 0 and a
// bare NUMERIC has no declared scale.
func declaredScale(typeToks []Token) (int, bool) {
	var numbers []int
	inParens := false
	for _, t := range typeToks {
		switch {
		case t.Kind == TokParen && t.Value == "(":
			inParens = true
		case t.Kind == TokNumber && inParens:
			n, _ := strconv.Atoi(t.Value)
			numbers = append(numbers, n)
		}
	}
	switch len(numbers) {
	case 1:
		return 0, true
	case 2:
		return numbers[1], true
	}
	return 0, false
}

func (p *numParser) parsePrimary() (numNode, bool) {
	start := p.pos
	i, ok := p.peek()
	if !ok {
		return numNode{}, false
	}
	t := p.toks[i]
	switch t.Kind {
	case TokNumber, TokString, TokParam:
		p.pos = i + 1
		return numNode{text: t.Raw}, true
	case TokParen:
		if t.Value != "(" {
			return numNode{}, false
		}
		p.pos = i + 1
		inner, ok := p.parseExpr()
		if !ok || !p.peekIs(TokParen, ")") {
			return numNode{}, false
		}
		p.take()
		return numNode{numeric: inner.numeric, rewritten: inner.rewritten, text: p.compose(start, inner.rewritten, "("+inner.text+")")}, true
	case TokKeyword:
		switch t.Value {
		case "NULL", "TRUE", "FALSE":
			p.pos = i + 1
			return numNode{text: t.Raw}, true
		case "CAST":
			return p.parseCast()
		case "COALESCE", "NULLIF":
			p.pos = i + 1
			if !p.peekIs(TokParen, "(") {
				return numNode{}, false
			}
			return p.parseCall(start, t.Raw)
		}
		return numNode{}, false
	case TokIdent:
		p.pos = i + 1
		for p.peekIs(TokDot, ".") {
			save := p.pos
			p.take()
			j, ok := p.peek()
			if !ok || p.toks[j].Kind != TokIdent {
				p.pos = save
				break
			}
			p.pos = j + 1
		}
		if p.peekIs(TokParen, "(") {
			return p.parseCall(start, p.span(start))
		}
		return numNode{text: p.span(start)}, true
	}
	return numNode{}, false
}

// parseCall parses the argument list at '('. round() with a numeric first
// argument becomes pg_numeric_round; any other call keeps its name and is
// numeric when an argument is, which is right for the pass-through
// functions (coalesce, nullif) and harmless elsewhere.
func (p *numParser) parseCall(start int, name string) (numNode, bool) {
	p.take()
	var args []numNode
	if !p.peekIs(TokParen, ")") {
		for {
			a, ok := p.parseExpr()
			if !ok {
				return numNode{}, false
			}
			args = append(args, a)
			if !p.peekIs(TokComma, ",") {
				break
			}
			p.take()
		}
	}
	if !p.peekIs(TokParen, ")") {
		return numNode{}, false
	}
	p.take()
	var numeric, rewritten bool
	texts := make([]string, len(args))
	for k, a := range args {
		numeric = numeric || a.numeric
		rewritten = rewritten || a.rewritten
		texts[k] = a.text
	}
	if strings.EqualFold(name, "round") && len(args) > 0 && args[0].numeric {
		name, rewritten = "pg_numeric_round", true
	}
	if agg, ok := numericAggregates[strings.ToLower(name)]; ok && len(args) == 1 && args[0].numeric {
		name, rewritten = agg, true
	}
	return numNode{numeric: numeric, rewritten: rewritten, text: p.compose(start, rewritten, name+"("+strings.Join(texts, ", ")+")")}, true
}

// parseCast parses CAST(expr AS type).
func (p *numParser) parseCast() (numNode, bool) {
	start := p.pos
	p.take()
	if !p.peekIs(TokParen, "(") {
		return numNode{}, false
	}
	p.take()
	inner, ok := p.parseExpr()
	if !ok || !p.peekIs(TokKeyword, "AS") {
		return numNode{}, false
	}
	p.take()
	i, ok := p.peek()
	if !ok || (p.toks[i].Kind != TokKeyword && p.toks[i].Kind != TokIdent) {
		return numNode{}, false
	}
	typeToks, end := extractTypeName(p.toks, i)
	p.pos = end + 1
	if !p.peekIs(TokParen, ")") {
		return numNode{}, false
	}
	p.take()
	typeName := assembleTypeName(typeToks)
	if typeName != "NUMERIC" && typeName != "DECIMAL" && !inner.rewritten {
		return numNode{text: p.span(start)}, true
	}
	return p.cast(start, inner, typeName, p.toks[i:end+1]), true
}
