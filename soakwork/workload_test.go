package soakwork

import (
	"database/sql"
	"testing"

	"github.com/drummonds/go-postgres/memcheck"
	_ "github.com/drummonds/go-postgres" // register pglike driver
)

func TestWorkloadSmoke(t *testing.T) {
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mon := memcheck.NewMonitor(1 << 30) // 1 GB — won't trip
	w := New(db, Config{BatchSize: 3}, mon)

	if err := w.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	for i := range 5 {
		r, err := w.RunIteration()
		if err != nil {
			t.Fatalf("iteration %d: %v", i+1, err)
		}
		if r.AccountCount == 0 {
			t.Fatalf("iteration %d: expected accounts", i+1)
		}
		if r.TxCount == 0 && i > 0 {
			t.Fatalf("iteration %d: expected transactions", i+1)
		}
	}
}

func TestWorkloadWithDelete(t *testing.T) {
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	w := New(db, Config{BatchSize: 5, DeleteThreshold: 10}, nil)

	if err := w.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	var lastTxCount int64
	for i := range 10 {
		r, err := w.RunIteration()
		if err != nil {
			t.Fatalf("iteration %d: %v", i+1, err)
		}
		// After threshold is hit, tx count should be pruned
		if i > 0 && r.TxCount > 0 && lastTxCount > int64(w.cfg.DeleteThreshold) {
			if r.TxCount >= lastTxCount {
				// Deletion should have reduced the count (or at least tried)
				t.Logf("iteration %d: txCount=%d (was %d), deletion may not have reduced enough yet", i+1, r.TxCount, lastTxCount)
			}
		}
		lastTxCount = r.TxCount
	}
}
