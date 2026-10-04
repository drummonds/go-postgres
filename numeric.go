package pglike

import (
	"errors"
	"math/big"
	"strconv"
	"strings"

	"github.com/ncruces/go-sqlite3"
)

// numeric is an exact decimal, coef × 10^-scale, the model PG's
// NUMERIC uses. It backs the pg_numeric_* functions the translator emits
// for expressions with a ::numeric operand, so SQLite evaluates them
// exactly and with PG's result scales instead of as REAL or as text.
type numeric struct {
	coef  *big.Int
	scale int
}

var errDivisionByZero = errors.New("division by zero")

func parseDecimal(s string) (numeric, error) {
	s = strings.TrimSpace(s)
	neg := false
	switch {
	case strings.HasPrefix(s, "-"):
		neg, s = true, s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	exp := 0
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		e, err := strconv.Atoi(s[i+1:])
		if err != nil {
			return numeric{}, errors.New("invalid input syntax for type numeric: " + strconv.Quote(s))
		}
		exp, s = e, s[:i]
	}
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i+1:]
	}
	digits := intPart + frac
	if digits == "" || strings.Trim(digits, "0123456789") != "" {
		return numeric{}, errors.New("invalid input syntax for type numeric: " + strconv.Quote(s))
	}
	coef, _ := new(big.Int).SetString(digits, 10)
	scale := len(frac) - exp
	if scale < 0 {
		coef.Mul(coef, pow10(-scale))
		scale = 0
	}
	if neg {
		coef.Neg(coef)
	}
	return numeric{coef, scale}, nil
}

func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

// String renders PG's text form: no exponent, every scale digit present,
// a leading zero before the point.
func (d numeric) String() string {
	s := new(big.Int).Abs(d.coef).String()
	if d.scale > 0 {
		if len(s) <= d.scale {
			s = strings.Repeat("0", d.scale-len(s)+1) + s
		}
		s = s[:len(s)-d.scale] + "." + s[len(s)-d.scale:]
	}
	if d.coef.Sign() < 0 {
		s = "-" + s
	}
	return s
}

// roundDiv divides a by b (b > 0) rounding half away from zero, as PG does.
func roundDiv(a, b *big.Int) *big.Int {
	q, r := new(big.Int).QuoRem(a, b, new(big.Int))
	twice := new(big.Int).Abs(r)
	twice.Mul(twice, big.NewInt(2))
	if twice.Cmp(b) >= 0 {
		if a.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return q
}

// rescale returns d at scale n, rounding when n is smaller.
func (d numeric) rescale(n int) numeric {
	if n >= d.scale {
		return numeric{new(big.Int).Mul(d.coef, pow10(n-d.scale)), n}
	}
	return numeric{roundDiv(d.coef, pow10(d.scale-n)), n}
}

// round is PG's round(numeric, n): scale exactly n; a negative n rounds to
// a power of ten and leaves scale zero.
func (d numeric) round(n int) numeric {
	if n < 0 {
		r := d.rescale(n)
		return numeric{r.coef.Mul(r.coef, pow10(-n)), 0}
	}
	return d.rescale(n)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (d numeric) add(o numeric) numeric {
	s := maxInt(d.scale, o.scale)
	a, b := d.rescale(s), o.rescale(s)
	return numeric{a.coef.Add(a.coef, b.coef), s}
}

func (d numeric) sub(o numeric) numeric {
	s := maxInt(d.scale, o.scale)
	a, b := d.rescale(s), o.rescale(s)
	return numeric{a.coef.Sub(a.coef, b.coef), s}
}

func (d numeric) mul(o numeric) numeric {
	return numeric{new(big.Int).Mul(d.coef, o.coef), d.scale + o.scale}
}

func (d numeric) neg() numeric {
	return numeric{new(big.Int).Neg(d.coef), d.scale}
}

func (d numeric) cmp(o numeric) int {
	s := maxInt(d.scale, o.scale)
	return d.rescale(s).coef.Cmp(o.rescale(s).coef)
}

// div follows PG's select_div_scale: at least 16 significant digits in
// the quotient, never fewer scale digits than either operand.
func (d numeric) div(o numeric) (numeric, error) {
	if o.coef.Sign() == 0 {
		return numeric{}, errDivisionByZero
	}
	rscale := selectDivScale(d, o)
	num, den := new(big.Int).Set(d.coef), new(big.Int).Set(o.coef)
	if e := rscale + o.scale - d.scale; e >= 0 {
		num.Mul(num, pow10(e))
	} else {
		den.Mul(den, pow10(-e))
	}
	if den.Sign() < 0 {
		num.Neg(num)
		den.Neg(den)
	}
	return numeric{roundDiv(num, den), rscale}, nil
}

func selectDivScale(a, b numeric) int {
	const minSigDigits, decDigits, maxDisplayScale = 16, 4, 1000
	w1, f1 := pgWeight(a)
	w2, f2 := pgWeight(b)
	qweight := w1 - w2
	if f1 <= f2 {
		qweight--
	}
	rscale := minSigDigits - qweight*decDigits
	rscale = maxInt(rscale, a.scale)
	rscale = maxInt(rscale, b.scale)
	rscale = maxInt(rscale, 0)
	if rscale > maxDisplayScale {
		rscale = maxDisplayScale
	}
	return rscale
}

// pgWeight is the weight and first digit of a number in PG's base-10000
// representation: weight is the position of the leading base-10000 digit
// relative to the point, first is that digit's value.
func pgWeight(d numeric) (weight, first int) {
	if d.coef.Sign() == 0 {
		return 0, 0
	}
	s := new(big.Int).Abs(d.coef).String()
	intPart, frac := s, ""
	if d.scale > 0 {
		if len(s) <= d.scale {
			s = strings.Repeat("0", d.scale-len(s)+1) + s
		}
		intPart, frac = s[:len(s)-d.scale], s[len(s)-d.scale:]
	}
	intPart = strings.TrimLeft(intPart, "0")
	if intPart != "" {
		n := len(intPart)
		weight = (n - 1) / 4
		first, _ = strconv.Atoi(intPart[:n-weight*4])
		return weight, first
	}
	for k := 0; 4*k < len(frac); k++ {
		chunk := frac[4*k:]
		if len(chunk) > 4 {
			chunk = chunk[:4]
		}
		chunk += strings.Repeat("0", 4-len(chunk))
		if v, _ := strconv.Atoi(chunk); v != 0 {
			return -(k + 1), v
		}
	}
	return 0, 0
}

// registerNumericFunctions installs pg_numeric and the pg_numeric_*
// operators on a connection. Every argument is read as text and parsed
// as a numeric, so integer, REAL and TEXT inputs all work; NULL in gives
// NULL out.
func registerNumericFunctions(conn *sqlite3.Conn) error {
	arg := func(v sqlite3.Value) (numeric, bool, error) {
		if v.Type() == sqlite3.NULL {
			return numeric{}, true, nil
		}
		d, err := parseDecimal(v.Text())
		return d, false, err
	}
	unary := func(name string, f func(numeric) numeric) error {
		return conn.CreateFunction(name, 1, sqlite3.DETERMINISTIC|sqlite3.INNOCUOUS,
			func(ctx sqlite3.Context, args ...sqlite3.Value) {
				a, null, err := arg(args[0])
				switch {
				case err != nil:
					ctx.ResultError(err)
				case null:
					ctx.ResultNull()
				default:
					ctx.ResultText(f(a).String())
				}
			})
	}
	binary := func(name string, f func(numeric, numeric) (numeric, error)) error {
		return conn.CreateFunction(name, 2, sqlite3.DETERMINISTIC|sqlite3.INNOCUOUS,
			func(ctx sqlite3.Context, args ...sqlite3.Value) {
				a, nullA, err := arg(args[0])
				if err != nil {
					ctx.ResultError(err)
					return
				}
				b, nullB, err := arg(args[1])
				if err != nil {
					ctx.ResultError(err)
					return
				}
				if nullA || nullB {
					ctx.ResultNull()
					return
				}
				r, err := f(a, b)
				if err != nil {
					ctx.ResultError(err)
					return
				}
				ctx.ResultText(r.String())
			})
	}
	exact := func(f func(numeric, numeric) numeric) func(numeric, numeric) (numeric, error) {
		return func(a, b numeric) (numeric, error) { return f(a, b), nil }
	}
	steps := []error{
		unary("pg_numeric", func(d numeric) numeric { return d }),
		unary("pg_numeric_neg", numeric.neg),
		binary("pg_numeric_add", exact(numeric.add)),
		binary("pg_numeric_sub", exact(numeric.sub)),
		binary("pg_numeric_mul", exact(numeric.mul)),
		binary("pg_numeric_div", numeric.div),
		conn.CreateFunction("pg_numeric_cmp", 2, sqlite3.DETERMINISTIC|sqlite3.INNOCUOUS,
			func(ctx sqlite3.Context, args ...sqlite3.Value) {
				a, nullA, err := arg(args[0])
				if err != nil {
					ctx.ResultError(err)
					return
				}
				b, nullB, err := arg(args[1])
				if err != nil {
					ctx.ResultError(err)
					return
				}
				if nullA || nullB {
					ctx.ResultNull()
					return
				}
				ctx.ResultInt(a.cmp(b))
			}),
		// round(numeric) and round(numeric, n)
		conn.CreateFunction("pg_numeric_round", -1, sqlite3.DETERMINISTIC|sqlite3.INNOCUOUS,
			func(ctx sqlite3.Context, args ...sqlite3.Value) {
				if len(args) == 0 || len(args) > 2 {
					ctx.ResultError(errors.New("pg_numeric_round takes one or two arguments"))
					return
				}
				a, null, err := arg(args[0])
				if err != nil {
					ctx.ResultError(err)
					return
				}
				n := 0
				if len(args) == 2 {
					if args[1].Type() == sqlite3.NULL {
						null = true
					}
					n = args[1].Int()
				}
				if null {
					ctx.ResultNull()
					return
				}
				ctx.ResultText(a.round(n).String())
			}),
	}
	for _, err := range steps {
		if err != nil {
			return err
		}
	}
	return nil
}
