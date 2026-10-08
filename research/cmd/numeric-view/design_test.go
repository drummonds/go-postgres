package main

import (
	"context"
	"database/sql"
	"testing"

	_ "git.bytestone.uk/hum3/go-postgres"
)

// Every design must load and publish its view on pglike; the int designs
// must be exact there. native-numeric's exactness on pglike is a study
// result, not a contract, so it is only required to run.
func TestDesignsRunOnPglike(t *testing.T) {
	ctx := context.Background()
	for _, d := range designs {
		t.Run(d.name, func(t *testing.T) {
			db, err := sql.Open("pglike", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			ms := generate(3, 10, 42)
			if _, err := load(ctx, db, d, ms, 4); err != nil {
				t.Fatalf("load: %v", err)
			}
			got, err := allBalances(ctx, db, d)
			if err != nil {
				t.Fatalf("all balances: %v", err)
			}
			if len(got) != 3 {
				t.Fatalf("want 3 balances, got %d", len(got))
			}
			one, err := oneBalance(ctx, db, d, 2)
			if err != nil {
				t.Fatalf("one balance: %v", err)
			}
			if one != got[2] {
				t.Fatalf("one-account %q differs from all-balances %q", one, got[2])
			}
			if d.name == "native-numeric" {
				return
			}
			if bad := checkExact(oracle(ms), got, d.scale); len(bad) != 0 {
				t.Fatalf("not exact: %v", bad)
			}
		})
	}
}

func TestGenerateIsDeterministicAndSpansAccounts(t *testing.T) {
	a, b := generate(5, 100, 7), generate(5, 100, 7)
	if len(a) != 500 || len(a) != len(b) {
		t.Fatalf("len %d %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("same seed must give same movements")
		}
	}
	seen := map[int64]bool{}
	for _, m := range a {
		seen[m.account] = true
	}
	if len(seen) != 5 {
		t.Fatalf("accounts seen %d want 5", len(seen))
	}
}
