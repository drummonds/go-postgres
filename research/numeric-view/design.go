package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"math/rand"
	"strings"
	"time"
)

// A movement is one ledger entry: an integer amount at exponent -7, so
// 1_0000000 is one unit. The native-numeric design stores the same value
// as a NUMERIC(20,7) literal.
type movement struct {
	account int64
	amount  int64
}

// A design is one way of storing the amount and one view that publishes
// the per-account balance. scale is the number of decimal places the view
// publishes: 0 for the integer control, 7 for the two numeric designs.
type design struct {
	name    string
	column  string // SQL type of movements.amount
	balance string // view expression over movements.amount
	scale   int
	encode  func(int64) any
}

const exponent = 7

var designs = []design{
	{
		name:    "int-control",
		column:  "BIGINT",
		balance: "SUM(amount)",
		scale:   0,
		encode:  func(a int64) any { return a },
	},
	{
		name:    "int-exponent",
		column:  "BIGINT",
		balance: "round(SUM(amount)::numeric / 10000000, 7)",
		scale:   exponent,
		encode:  func(a int64) any { return a },
	},
	{
		name:    "native-numeric",
		column:  "NUMERIC(20,7)",
		balance: "SUM(amount)",
		scale:   exponent,
		encode:  func(a int64) any { return scaledString(big.NewInt(a), exponent) },
	},
}

func (d design) table() string { return "movements_" + strings.ReplaceAll(d.name, "-", "_") }
func (d design) view() string  { return "balances_" + strings.ReplaceAll(d.name, "-", "_") }

func (d design) ddl() []string {
	return []string{
		fmt.Sprintf("DROP VIEW IF EXISTS %s", d.view()),
		fmt.Sprintf("DROP TABLE IF EXISTS %s", d.table()),
		fmt.Sprintf("CREATE TABLE %s (id BIGINT NOT NULL, account_id BIGINT NOT NULL, amount %s NOT NULL)", d.table(), d.column),
		fmt.Sprintf("CREATE INDEX %s_account ON %s (account_id)", d.table(), d.table()),
		fmt.Sprintf("CREATE VIEW %s AS SELECT account_id, %s AS balance FROM %s GROUP BY account_id", d.view(), d.balance, d.table()),
	}
}

// generate makes entries movements for each of n accounts, interleaved by
// account as a ledger would receive them, from a fixed seed. Amounts span
// ±100 units with every one of the 7 fractional digits in play.
func generate(n, entries int, seed int64) []movement {
	r := rand.New(rand.NewSource(seed))
	ms := make([]movement, 0, n*entries)
	for e := 0; e < entries; e++ {
		for a := 1; a <= n; a++ {
			ms = append(ms, movement{int64(a), r.Int63n(200_0000001) - 100_0000000})
		}
	}
	return ms
}

// load creates the design's table and view and inserts the movements in
// multi-row batches through one prepared statement, in one transaction.
// Both drivers take the identical path.
func load(ctx context.Context, db *sql.DB, d design, ms []movement, batch int) (time.Duration, error) {
	start := time.Now()
	for _, s := range d.ddl() {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return 0, fmt.Errorf("%s: %w", s, err)
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, insertSQL(d, batch))
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	args := make([]any, 0, 3*batch)
	for i := 0; i < len(ms); i += batch {
		end := min(i+batch, len(ms))
		if end-i != batch {
			stmt.Close()
			if stmt, err = tx.PrepareContext(ctx, insertSQL(d, end-i)); err != nil {
				return 0, err
			}
		}
		args = args[:0]
		for j := i; j < end; j++ {
			args = append(args, int64(j+1), ms[j].account, d.encode(ms[j].amount))
		}
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return 0, fmt.Errorf("insert: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func insertSQL(d design, rows int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "INSERT INTO %s (id, account_id, amount) VALUES ", d.table())
	for i := 0; i < rows; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "($%d, $%d, $%d)", 3*i+1, 3*i+2, 3*i+3)
	}
	return b.String()
}

// allBalances reads the whole view, as a reporting query would.
func allBalances(ctx context.Context, db *sql.DB, d design) (map[int64]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT account_id, balance FROM "+d.view())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var acct int64
		var bal string
		if err := rows.Scan(&acct, &bal); err != nil {
			return nil, err
		}
		out[acct] = bal
	}
	return out, rows.Err()
}

// oneBalance reads one account through the view, as a balance lookup would.
func oneBalance(ctx context.Context, db *sql.DB, d design, account int64) (string, error) {
	var bal string
	err := db.QueryRowContext(ctx, "SELECT balance FROM "+d.view()+" WHERE account_id = $1", account).Scan(&bal)
	return bal, err
}
