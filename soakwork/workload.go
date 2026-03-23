package soakwork

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"codeberg.org/hum3/go-postgres/memcheck"
)

// Config controls the soak workload parameters.
type Config struct {
	BatchSize       int           // accounts per iteration (default 10)
	DeleteThreshold int           // delete oldest 10% of txns when count exceeds this (0 = never delete)
	ReportInterval  time.Duration // how often to call reportFn (default 10s)
}

func (c *Config) defaults() {
	if c.BatchSize <= 0 {
		c.BatchSize = 10
	}
	if c.ReportInterval <= 0 {
		c.ReportInterval = 10 * time.Second
	}
}

// Report holds per-iteration statistics.
type Report struct {
	Iteration    int
	AccountCount int64
	TxCount      int64
	Stats        memcheck.Stats
	OpsPerSec    float64
	Elapsed      time.Duration
}

// Workload drives a sustained CRUD workload against a pglike database.
type Workload struct {
	db        *sql.DB
	cfg       Config
	monitor   *memcheck.Monitor
	iteration int
	start     time.Time
	ops       int
}

// New creates a Workload.
func New(db *sql.DB, cfg Config, monitor *memcheck.Monitor) *Workload {
	cfg.defaults()
	return &Workload{
		db:      db,
		cfg:     cfg,
		monitor: monitor,
	}
}

// Setup creates the schema tables.
func (w *Workload) Setup() error {
	_, err := w.db.Exec(`
		CREATE TABLE IF NOT EXISTS accounts (
			id UUID DEFAULT (gen_random_uuid()) PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			balance BIGINT NOT NULL DEFAULT 0,
			interest_rate BIGINT NOT NULL DEFAULT 500,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create accounts: %w", err)
	}
	_, err = w.db.Exec(`
		CREATE TABLE IF NOT EXISTS transactions (
			id UUID DEFAULT (gen_random_uuid()) PRIMARY KEY,
			account_id UUID REFERENCES accounts(id),
			amount BIGINT NOT NULL,
			description TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create transactions: %w", err)
	}
	return nil
}

// RunIteration executes one CRUD round and returns a report.
func (w *Workload) RunIteration() (Report, error) {
	if w.start.IsZero() {
		w.start = time.Now()
	}
	w.iteration++

	// 1. INSERT batch of accounts
	for i := range w.cfg.BatchSize {
		_, err := w.db.Exec(
			"INSERT INTO accounts (name, balance, interest_rate) VALUES ($1, $2, $3)",
			fmt.Sprintf("acct-%d-%d", w.iteration, i),
			int64(100000+i*1000), // starting balance in cents
			int64(300+i*10),      // interest rate basis points
		)
		if err != nil {
			return Report{}, fmt.Errorf("insert account: %w", err)
		}
		w.ops++
	}

	// 2. Interest calculation: SELECT each account, compute interest, INSERT transaction, UPDATE balance
	rows, err := w.db.Query("SELECT id, balance, interest_rate FROM accounts")
	if err != nil {
		return Report{}, fmt.Errorf("select accounts: %w", err)
	}
	type acctRow struct {
		id           string
		balance      int64
		interestRate int64
	}
	var accts []acctRow
	for rows.Next() {
		var a acctRow
		if err := rows.Scan(&a.id, &a.balance, &a.interestRate); err != nil {
			rows.Close()
			return Report{}, fmt.Errorf("scan account: %w", err)
		}
		accts = append(accts, a)
	}
	rows.Close()
	w.ops++

	for _, a := range accts {
		interest := a.balance * a.interestRate / 365 / 10000
		if interest == 0 {
			interest = 1
		}
		_, err := w.db.Exec(
			"INSERT INTO transactions (account_id, amount, description) VALUES ($1, $2, $3)",
			a.id, interest, "daily interest",
		)
		if err != nil {
			return Report{}, fmt.Errorf("insert transaction: %w", err)
		}
		_, err = w.db.Exec(
			"UPDATE accounts SET balance = balance + $1 WHERE id = $2",
			interest, a.id,
		)
		if err != nil {
			return Report{}, fmt.Errorf("update balance: %w", err)
		}
		w.ops += 2
	}

	// 3. Aggregate stats
	var accountCount, txCount int64
	err = w.db.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&accountCount)
	if err != nil {
		return Report{}, fmt.Errorf("count accounts: %w", err)
	}
	err = w.db.QueryRow("SELECT COUNT(*) FROM transactions").Scan(&txCount)
	if err != nil {
		return Report{}, fmt.Errorf("count transactions: %w", err)
	}
	w.ops += 2

	// 4. Delete oldest 10% of transactions if over threshold
	if w.cfg.DeleteThreshold > 0 && txCount > int64(w.cfg.DeleteThreshold) {
		deleteCount := txCount / 10
		_, err = w.db.Exec(
			"DELETE FROM transactions WHERE id IN (SELECT id FROM transactions ORDER BY created_at ASC LIMIT $1)",
			deleteCount,
		)
		if err != nil {
			return Report{}, fmt.Errorf("delete transactions: %w", err)
		}
		w.ops++
	}

	elapsed := time.Since(w.start)
	var opsPerSec float64
	if elapsed.Seconds() > 0 {
		opsPerSec = float64(w.ops) / elapsed.Seconds()
	}

	var stats memcheck.Stats
	if w.monitor != nil {
		_, stats = w.monitor.Check()
	} else {
		stats = memcheck.ReadStats()
	}

	return Report{
		Iteration:    w.iteration,
		AccountCount: accountCount,
		TxCount:      txCount,
		Stats:        stats,
		OpsPerSec:    opsPerSec,
		Elapsed:      elapsed,
	}, nil
}

// Run loops RunIteration until ctx is cancelled or the memory monitor trips.
// It calls reportFn at ReportInterval with the latest report.
func (w *Workload) Run(ctx context.Context, reportFn func(Report)) error {
	lastReport := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if w.monitor != nil && w.monitor.Exceeded() {
			return fmt.Errorf("memory limit exceeded")
		}

		r, err := w.RunIteration()
		if err != nil {
			return err
		}

		if reportFn != nil && time.Since(lastReport) >= w.cfg.ReportInterval {
			reportFn(r)
			lastReport = time.Now()
		}
	}
}
