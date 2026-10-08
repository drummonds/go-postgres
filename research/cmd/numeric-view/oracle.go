package main

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
)

// oracle is the exact per-account integer sum, computed in Go.
func oracle(ms []movement) map[int64]*big.Int {
	out := map[int64]*big.Int{}
	for _, m := range ms {
		s, ok := out[m.account]
		if !ok {
			s = new(big.Int)
			out[m.account] = s
		}
		s.Add(s, big.NewInt(m.amount))
	}
	return out
}

// scaledString renders coef × 10^-scale as PG would print a NUMERIC.
func scaledString(coef *big.Int, scale int) string {
	s := new(big.Int).Abs(coef).String()
	if scale > 0 {
		if len(s) <= scale {
			s = strings.Repeat("0", scale-len(s)+1) + s
		}
		s = s[:len(s)-scale] + "." + s[len(s)-scale:]
	}
	if coef.Sign() < 0 {
		s = "-" + s
	}
	return s
}

// parseDecimal reads any decimal text a driver might return, including an
// exponent form, exactly.
func parseDecimal(s string) (*big.Rat, error) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		return nil, fmt.Errorf("not a decimal: %q", s)
	}
	return r, nil
}

type mismatch struct {
	account   int64
	want, got string
}

func (m mismatch) String() string {
	return fmt.Sprintf("account %d: want %s got %s", m.account, m.want, m.got)
}

// checkExact compares what a view published with the oracle at the
// design's scale. A value passes if it is numerically equal, whatever its
// text form; a missing account is a mismatch.
func checkExact(want map[int64]*big.Int, got map[int64]string, scale int) []mismatch {
	var bad []mismatch
	for acct, coef := range want {
		exp := new(big.Rat).SetFrac(coef, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil))
		text, ok := got[acct]
		if !ok {
			bad = append(bad, mismatch{acct, scaledString(coef, scale), "<missing>"})
			continue
		}
		v, err := parseDecimal(text)
		if err != nil || v.Cmp(exp) != 0 {
			bad = append(bad, mismatch{acct, scaledString(coef, scale), text})
		}
	}
	sort.Slice(bad, func(i, j int) bool { return bad[i].account < bad[j].account })
	return bad
}
