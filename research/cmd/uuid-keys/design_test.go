package main

import (
	"context"
	"regexp"
	"testing"

	"git.bytestone.uk/hum3/go-postgres/research/internal/harness"
)

func TestUUIDv4IsDeterministicAndWellFormed(t *testing.T) {
	a, b := newIDs(42), newIDs(42)
	x, y := a.next(), b.next()
	if x != y {
		t.Fatalf("same seed must give same ids: %s %s", x, y)
	}
	if x == a.next() {
		t.Fatal("consecutive ids must differ")
	}
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !re.MatchString(x) {
		t.Fatalf("not a canonical v4 uuid: %s", x)
	}
}

func TestGenerateLinksEveryMovementToAnAccount(t *testing.T) {
	accts, moves := generate(3, 10, 7)
	if len(accts) != 3 || len(moves) != 30 {
		t.Fatalf("got %d accounts, %d movements", len(accts), len(moves))
	}
	known := map[string]bool{}
	for _, a := range accts {
		known[a.id] = true
	}
	for _, m := range moves {
		if !known[m.account] {
			t.Fatalf("movement %s references unknown account %s", m.id, m.account)
		}
	}
}

// Every design that is not Postgres-only must load with its foreign key
// and answer the three study queries on pglike.
func TestDesignsRunOnPglike(t *testing.T) {
	ctx := context.Background()
	b := harness.Backends(t.TempDir())[0]
	for _, d := range designs {
		if d.pgOnly {
			continue
		}
		t.Run(d.name, func(t *testing.T) {
			db, closeDB, err := b.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer closeDB()
			accts, moves := generate(3, 10, 42)
			if _, err := load(ctx, db, d, accts, moves, 4); err != nil {
				t.Fatalf("load: %v", err)
			}
			n, _, err := byAccount(ctx, db, d, accts[1].id)
			if err != nil || n != 10 {
				t.Fatalf("by account: n=%d err=%v", n, err)
			}
			if _, err := byID(ctx, db, d, moves[5].id); err != nil {
				t.Fatalf("by id: %v", err)
			}
			if n, err := join(ctx, db, d); err != nil || n != 30 {
				t.Fatalf("join: n=%d err=%v", n, err)
			}
			if sz := harness.FileSize(db, b.Name); sz <= 0 {
				t.Fatalf("file size %d", sz)
			}
			if _, err := db.ExecContext(ctx, "INSERT INTO "+d.movements()+" VALUES ($1, $2, 1)", "x", "no-such-account"); err == nil {
				t.Fatal("foreign key not enforced")
			}
		})
	}
}
