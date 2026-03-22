package pglike

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// SoakMetrics holds a single metrics snapshot.
type SoakMetrics struct {
	Timestamp    time.Time `json:"timestamp"`
	ElapsedSecs  float64   `json:"elapsed_secs"`
	HeapAllocMB  float64   `json:"heap_alloc_mb"`
	HeapSysMB    float64   `json:"heap_sys_mb"`
	NumGC        uint32    `json:"num_gc"`
	Goroutines   int       `json:"goroutines"`
	TotalOps     int64     `json:"total_ops"`
	TotalErrors  int64     `json:"total_errors"`
	OpsPerSec    float64   `json:"ops_per_sec"`
	AvgLatencyUs float64   `json:"avg_latency_us"`
}

// soakConfig holds soak test parameters, all from env vars.
type soakConfig struct {
	Duration       time.Duration
	Workers        int
	MetricInterval time.Duration
	OutputFile     string
}

func loadSoakConfig() soakConfig {
	c := soakConfig{
		Duration:       1 * time.Minute,
		Workers:        4,
		MetricInterval: 5 * time.Second,
		OutputFile:     "",
	}
	if v := os.Getenv("SOAK_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.Duration = d
		}
	}
	if v := os.Getenv("SOAK_WORKERS"); v != "" {
		var w int
		if _, err := fmt.Sscanf(v, "%d", &w); err == nil && w > 0 {
			c.Workers = w
		}
	}
	if v := os.Getenv("SOAK_METRIC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.MetricInterval = d
		}
	}
	if v := os.Getenv("SOAK_OUTPUT"); v != "" {
		c.OutputFile = v
	}
	return c
}

// TestSoak runs a sustained workload against the pglike driver.
//
// Env vars:
//
//	SOAK_DURATION        - test duration (default 1m, e.g. "1h", "30m")
//	SOAK_WORKERS         - concurrent goroutines (default 4)
//	SOAK_METRIC_INTERVAL - metrics sampling interval (default 5s)
//	SOAK_OUTPUT          - JSON lines output file (default: stdout)
//
// Run: go test -run TestSoak -timeout 0 -count=1
func TestSoak(t *testing.T) {
	if os.Getenv("SOAK_DURATION") == "" {
		t.Skip("skipping soak test: set SOAK_DURATION to enable (e.g. SOAK_DURATION=1m)")
	}

	cfg := loadSoakConfig()
	t.Logf("soak config: duration=%s workers=%d interval=%s", cfg.Duration, cfg.Workers, cfg.MetricInterval)

	db := openTestDB(t)
	db.SetMaxOpenConns(cfg.Workers + 2)

	setupSoakSchema(t, db)

	// Metrics output
	var out *json.Encoder
	if cfg.OutputFile != "" {
		f, err := os.Create(cfg.OutputFile)
		if err != nil {
			t.Fatalf("create output file: %v", err)
		}
		defer f.Close()
		out = json.NewEncoder(f)
	} else {
		out = json.NewEncoder(os.Stdout)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	var totalOps, totalErrors atomic.Int64
	var totalLatencyNs atomic.Int64
	start := time.Now()

	// Metrics collector
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(cfg.MetricInterval)
		defer ticker.Stop()
		var lastOps int64
		for {
			select {
			case <-ctx.Done():
				// Final snapshot
				emitMetrics(out, start, &totalOps, &totalErrors, &totalLatencyNs, lastOps)
				return
			case <-ticker.C:
				lastOps = emitMetrics(out, start, &totalOps, &totalErrors, &totalLatencyNs, lastOps)
			}
		}
	}()

	// Workers
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
			soakWorker(ctx, db, workerID, rng, &totalOps, &totalErrors, &totalLatencyNs)
		}(i)
	}

	wg.Wait()

	ops := totalOps.Load()
	errs := totalErrors.Load()
	elapsed := time.Since(start)
	t.Logf("soak complete: %d ops, %d errors (%.2f%%), %.0f ops/sec over %s",
		ops, errs, float64(errs)/float64(ops+1)*100, float64(ops)/elapsed.Seconds(), elapsed)

	if errs > 0 {
		t.Errorf("soak test had %d errors out of %d ops", errs, ops)
	}
}

func setupSoakSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS soak_users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			email VARCHAR(200),
			active BOOLEAN DEFAULT TRUE,
			score INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS soak_events (
			id SERIAL PRIMARY KEY,
			user_id INTEGER REFERENCES soak_users(id),
			kind VARCHAR(50) NOT NULL,
			payload TEXT,
			created_at TIMESTAMP DEFAULT (datetime('now'))
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("schema setup: %v", err)
		}
	}
}

func soakWorker(ctx context.Context, db *sql.DB, _ int, rng *rand.Rand, ops, errs, latNs *atomic.Int64) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		var err error

		switch rng.Intn(10) {
		case 0, 1, 2: // 30% inserts
			err = soakInsertUser(ctx, db, rng)
		case 3, 4: // 20% insert events
			err = soakInsertEvent(ctx, db, rng)
		case 5, 6, 7: // 30% reads
			err = soakQuery(ctx, db, rng)
		case 8: // 10% updates
			err = soakUpdate(ctx, db, rng)
		case 9: // 10% transactions
			err = soakTransaction(ctx, db, rng)
		}

		elapsed := time.Since(start)
		latNs.Add(elapsed.Nanoseconds())
		ops.Add(1)
		if err != nil && ctx.Err() == nil {
			errs.Add(1)
		}
	}
}

func soakInsertUser(ctx context.Context, db *sql.DB, rng *rand.Rand) error {
	name := fmt.Sprintf("user_%d", rng.Intn(100000))
	email := fmt.Sprintf("%s@example.com", name)
	_, err := db.ExecContext(ctx,
		"INSERT INTO soak_users (name, email, score) VALUES (?, ?, ?)",
		name, email, rng.Intn(1000))
	return err
}

func soakInsertEvent(ctx context.Context, db *sql.DB, rng *rand.Rand) error {
	// Get a valid user_id or skip if none exist yet.
	var userID int64
	err := db.QueryRowContext(ctx, "SELECT id FROM soak_users ORDER BY RANDOM() LIMIT 1").Scan(&userID)
	if err != nil {
		return nil // no users yet, not an error
	}
	kinds := []string{"login", "logout", "purchase", "view", "click"}
	_, err = db.ExecContext(ctx,
		"INSERT INTO soak_events (user_id, kind, payload) VALUES (?, ?, ?)",
		userID, kinds[rng.Intn(len(kinds))], fmt.Sprintf(`{"worker":%d}`, rng.Intn(100)))
	return err
}

func soakQuery(ctx context.Context, db *sql.DB, rng *rand.Rand) error {
	switch rng.Intn(4) {
	case 0: // count
		var count int
		return db.QueryRowContext(ctx, "SELECT count(*) FROM soak_users").Scan(&count)
	case 1: // filtered read
		rows, err := db.QueryContext(ctx,
			"SELECT id, name, score FROM soak_users WHERE score > ? LIMIT 10", rng.Intn(500))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var name string
			var score int
			if err := rows.Scan(&id, &name, &score); err != nil {
				return err
			}
		}
		return rows.Err()
	case 2: // join query
		rows, err := db.QueryContext(ctx,
			`SELECT u.name, e.kind, e.created_at
			 FROM soak_users u
			 JOIN soak_events e ON e.user_id = u.id
			 ORDER BY e.created_at DESC LIMIT 5`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name, kind, ts string
			if err := rows.Scan(&name, &kind, &ts); err != nil {
				return err
			}
		}
		return rows.Err()
	case 3: // gen_random_uuid
		var uuid string
		return db.QueryRowContext(ctx, "SELECT gen_random_uuid()").Scan(&uuid)
	}
	return nil
}

func soakUpdate(ctx context.Context, db *sql.DB, rng *rand.Rand) error {
	_, err := db.ExecContext(ctx,
		"UPDATE soak_users SET score = score + ? WHERE id IN (SELECT id FROM soak_users ORDER BY RANDOM() LIMIT 1)",
		rng.Intn(10))
	return err
}

func soakTransaction(ctx context.Context, db *sql.DB, rng *rand.Rand) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	name := fmt.Sprintf("txuser_%d", rng.Intn(100000))
	res, err := tx.ExecContext(ctx,
		"INSERT INTO soak_users (name, email, score) VALUES (?, ?, ?)",
		name, name+"@tx.com", rng.Intn(1000))
	if err != nil {
		return err
	}
	userID, _ := res.LastInsertId()

	_, err = tx.ExecContext(ctx,
		"INSERT INTO soak_events (user_id, kind, payload) VALUES (?, ?, ?)",
		userID, "signup", `{"via":"soak_tx"}`)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func emitMetrics(enc *json.Encoder, start time.Time, ops, errs, latNs *atomic.Int64, prevOps int64) int64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	curOps := ops.Load()
	elapsed := time.Since(start).Seconds()
	avgLat := float64(0)
	if curOps > 0 {
		avgLat = float64(latNs.Load()) / float64(curOps) / 1000 // microseconds
	}

	snap := SoakMetrics{
		Timestamp:    time.Now().UTC(),
		ElapsedSecs:  elapsed,
		HeapAllocMB:  float64(m.HeapAlloc) / 1024 / 1024,
		HeapSysMB:    float64(m.HeapSys) / 1024 / 1024,
		NumGC:        m.NumGC,
		Goroutines:   runtime.NumGoroutine(),
		TotalOps:     curOps,
		TotalErrors:  errs.Load(),
		OpsPerSec:    float64(curOps-prevOps) / elapsed * (elapsed / (elapsed + 0.001)), // approximate interval rate
		AvgLatencyUs: avgLat,
	}

	// Fix OpsPerSec to be interval-based
	if elapsed > 0 {
		snap.OpsPerSec = float64(curOps) / elapsed
	}

	enc.Encode(snap)
	return curOps
}
