package main

import (
	"math/big"
	"testing"
)

func TestOracleSumsPerAccountExactly(t *testing.T) {
	ms := []movement{{1, 1_0000000}, {1, 5}, {2, -3}, {1, -1_0000000}}
	o := oracle(ms)
	if got := o[1].String(); got != "5" {
		t.Fatalf("account 1: got %s want 5", got)
	}
	if got := o[2].String(); got != "-3" {
		t.Fatalf("account 2: got %s want -3", got)
	}
}

func TestParseDecimalExactly(t *testing.T) {
	cases := map[string]*big.Rat{
		"123.4567890":  big.NewRat(1234567890, 10000000),
		"-0.0000001":   big.NewRat(-1, 10000000),
		"42":           big.NewRat(42, 1),
		"1.5e6":        big.NewRat(1500000, 1),
		"1234567.89e0": big.NewRat(123456789, 100),
	}
	for s, want := range cases {
		got, err := parseDecimal(s)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		if got.Cmp(want) != 0 {
			t.Errorf("%q: got %s want %s", s, got, want)
		}
	}
	if _, err := parseDecimal("abc"); err == nil {
		t.Error("abc: want error")
	}
}

// checkExact compares what a view returned with the oracle. The int-exponent
// and native-numeric designs publish amount × 10^-7; int-control publishes
// the raw integer sum. The design says how the oracle's integer maps to the
// published value.
func TestCheckExactReportsMismatches(t *testing.T) {
	o := map[int64]*big.Int{1: big.NewInt(12345678901), 2: big.NewInt(-5)}
	scaled := map[int64]string{1: "1234.5678901", 2: "-0.0000005"}
	if bad := checkExact(o, scaled, 7); len(bad) != 0 {
		t.Fatalf("exact rows reported as mismatches: %v", bad)
	}
	lossy := map[int64]string{1: "1234.5678901", 2: "-5e-07"}
	if bad := checkExact(o, lossy, 7); len(bad) != 0 {
		t.Fatalf("an exact float rendering must pass: %v", bad)
	}
	wrong := map[int64]string{1: "1234.56789", 2: "-0.0000005"}
	if bad := checkExact(o, wrong, 7); len(bad) != 1 || bad[0].account != 1 {
		t.Fatalf("truncated row not reported: %v", bad)
	}
	missing := map[int64]string{1: "1234.5678901"}
	if bad := checkExact(o, missing, 7); len(bad) != 1 || bad[0].account != 2 {
		t.Fatalf("missing row not reported: %v", bad)
	}
	raw := map[int64]string{1: "12345678901", 2: "-5"}
	if bad := checkExact(o, raw, 0); len(bad) != 0 {
		t.Fatalf("scale 0 compares the raw integer: %v", bad)
	}
}
