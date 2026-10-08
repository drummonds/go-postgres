package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"git.bytestone.uk/hum3/go-postgres/research/internal/harness"
)

// A design is the column type used for every key: the accounts primary
// key, the movements primary key and the movements foreign key.
type design struct {
	name    string
	keyType string
	pgOnly  bool
}

var designs = []design{
	{name: "uuid", keyType: "UUID"},
	{name: "text", keyType: "TEXT"},
	{name: "text-c", keyType: `TEXT COLLATE "C"`, pgOnly: true},
}

func (d design) suffix() string    { return strings.ReplaceAll(d.name, "-", "_") }
func (d design) accounts() string  { return "accounts_" + d.suffix() }
func (d design) movements() string { return "movements_" + d.suffix() }

func (d design) ddl() []string {
	return []string{
		fmt.Sprintf("DROP TABLE IF EXISTS %s", d.movements()),
		fmt.Sprintf("DROP TABLE IF EXISTS %s", d.accounts()),
		fmt.Sprintf("CREATE TABLE %s (id %s PRIMARY KEY, ref VARCHAR(20) NOT NULL)", d.accounts(), d.keyType),
		fmt.Sprintf("CREATE TABLE %s (id %s PRIMARY KEY, account_id %s NOT NULL REFERENCES %s(id), amount BIGINT NOT NULL)",
			d.movements(), d.keyType, d.keyType, d.accounts()),
		fmt.Sprintf("CREATE INDEX %s_account ON %s (account_id)", d.movements(), d.movements()),
	}
}

type account struct {
	id, ref string
}

type movement struct {
	id, account string
	amount      int64
}

// ids yields canonical v4 UUID strings from a seeded source, so every
// design and every run sees the same keys.
type ids struct{ r *rand.Rand }

func newIDs(seed int64) *ids { return &ids{rand.New(rand.NewSource(seed))} }

func (g *ids) next() string {
	var b [16]byte
	g.r.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// generate makes n accounts and entries movements per account, interleaved
// by account as a ledger would receive them.
func generate(n, entries int, seed int64) ([]account, []movement) {
	g := newIDs(seed)
	r := rand.New(rand.NewSource(seed + 1))
	accts := make([]account, n)
	for i := range accts {
		accts[i] = account{id: g.next(), ref: fmt.Sprintf("ACC%07d", i+1)}
	}
	moves := make([]movement, 0, n*entries)
	for e := 0; e < entries; e++ {
		for _, a := range accts {
			moves = append(moves, movement{id: g.next(), account: a.id, amount: r.Int63n(200_0000001) - 100_0000000})
		}
	}
	return accts, moves
}

// load creates the design's tables and inserts accounts then movements in
// multi-row batches through prepared statements, in one transaction.
func load(ctx context.Context, db *sql.DB, d design, accts []account, moves []movement, batch int) (time.Duration, error) {
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
	accountRows := make([][]any, len(accts))
	for i, a := range accts {
		accountRows[i] = []any{a.id, a.ref}
	}
	if err := insertBatched(ctx, tx, d.accounts(), "id, ref", accountRows, batch); err != nil {
		return 0, err
	}
	moveRows := make([][]any, len(moves))
	for i, m := range moves {
		moveRows[i] = []any{m.id, m.account, m.amount}
	}
	if err := insertBatched(ctx, tx, d.movements(), "id, account_id, amount", moveRows, batch); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func insertBatched(ctx context.Context, tx *sql.Tx, table, columns string, rows [][]any, batch int) error {
	if len(rows) == 0 {
		return nil
	}
	width := len(rows[0])
	prepare := func(n int) (*sql.Stmt, error) {
		var b strings.Builder
		fmt.Fprintf(&b, "INSERT INTO %s (%s) VALUES ", table, columns)
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString("(")
			for j := 0; j < width; j++ {
				if j > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "$%d", i*width+j+1)
			}
			b.WriteString(")")
		}
		return tx.PrepareContext(ctx, b.String())
	}
	stmt, err := prepare(batch)
	if err != nil {
		return err
	}
	defer stmt.Close()
	args := make([]any, 0, width*batch)
	for i := 0; i < len(rows); i += batch {
		end := min(i+batch, len(rows))
		if end-i != batch {
			stmt.Close()
			if stmt, err = prepare(end - i); err != nil {
				return err
			}
		}
		args = args[:0]
		for _, r := range rows[i:end] {
			args = append(args, r...)
		}
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return fmt.Errorf("insert %s: %w", table, err)
		}
	}
	return nil
}

// byAccount walks the foreign-key index: one account's movement count and
// balance.
func byAccount(ctx context.Context, db *sql.DB, d design, account string) (int64, int64, error) {
	var n, sum int64
	err := db.QueryRowContext(ctx, "SELECT count(*), coalesce(sum(amount), 0) FROM "+d.movements()+" WHERE account_id = $1", account).Scan(&n, &sum)
	return n, sum, err
}

// byID is a primary-key point lookup.
func byID(ctx context.Context, db *sql.DB, d design, id string) (int64, error) {
	var amount int64
	err := db.QueryRowContext(ctx, "SELECT amount FROM "+d.movements()+" WHERE id = $1", id).Scan(&amount)
	return amount, err
}

// join matches every movement to its account on the key.
func join(ctx context.Context, db *sql.DB, d design) (int64, error) {
	var n int64
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+d.movements()+" m JOIN "+d.accounts()+" a ON a.id = m.account_id WHERE a.ref <> ''").Scan(&n)
	return n, err
}

// sizes reports table and index bytes for the movements table on Postgres,
// or the whole database file on pglike-file.
func sizes(ctx context.Context, db *sql.DB, backend string, d design) (table, index int64) {
	if backend == "postgres" {
		db.QueryRowContext(ctx, "SELECT pg_table_size($1), pg_indexes_size($1)", d.movements()).Scan(&table, &index)
		return
	}
	return harness.FileSize(db, backend), 0
}
