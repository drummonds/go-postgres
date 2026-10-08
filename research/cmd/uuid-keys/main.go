// Command uuid-keys measures what declaring UUID keys as TEXT costs on
// PostgreSQL, with pglike as the control where both are TEXT. It writes
// its results between the result markers of research-uuid-keys.md.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"git.bytestone.uk/hum3/go-postgres/research/internal/harness"
)

type result struct {
	backend, design     string
	accounts, rows      int
	load                time.Duration
	table, index        int64
	byAccount, byID, jn harness.Stats
}

func main() {
	scalesFlag := flag.String("scales", "1000,10000,100000", "comma-separated account counts")
	entries := flag.Int("entries", 100, "movements per account")
	batch := flag.Int("batch", 500, "rows per INSERT statement")
	itersPoint := flag.Int("iters-point", 200, "iterations of the by-account and by-id queries")
	itersJoin := flag.Int("iters-join", 3, "iterations of the join")
	backendsFlag := flag.String("backends", "pglike-file,postgres", "backends to run")
	out := flag.String("out", "../research-uuid-keys.md", "document to splice results into (empty: stdout only)")
	seed := flag.Int64("seed", 42, "data seed")
	flag.Parse()

	var scales []int
	for _, s := range strings.Split(*scalesFlag, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			log.Fatalf("bad scale %q", s)
		}
		scales = append(scales, n)
	}
	want := map[string]bool{}
	for _, b := range strings.Split(*backendsFlag, ",") {
		want[strings.TrimSpace(b)] = true
	}
	tmp, err := os.MkdirTemp("", "uuid-keys")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	ctx := context.Background()
	var results []result
	var skipped []string
	versions := map[string]string{}
	for _, b := range harness.Backends(tmp) {
		if !want[b.Name] {
			continue
		}
		for _, n := range scales {
			accts, moves := generate(n, *entries, *seed)
			for _, d := range designs {
				if d.pgOnly && b.Name != "postgres" {
					continue
				}
				// A fresh database per design so a file size is one design's.
				db, closeDB, err := b.Open()
				if err != nil {
					skipped = append(skipped, fmt.Sprintf("%s: %v", b.Name, err))
					log.Printf("skip %s: %v", b.Name, err)
					break
				}
				versions[b.Name] = harness.ServerVersion(ctx, db)
				log.Printf("%s %s n=%d", b.Name, d.name, n)
				r, err := run(ctx, db, b.Name, d, accts, moves, *batch, *itersPoint, *itersJoin, *seed)
				if err != nil {
					log.Fatalf("%s %s n=%d: %v", b.Name, d.name, n, err)
				}
				results = append(results, r)
				db.ExecContext(ctx, "DROP TABLE IF EXISTS "+d.movements())
				db.ExecContext(ctx, "DROP TABLE IF EXISTS "+d.accounts())
				closeDB()
			}
		}
	}

	md := render(results, skipped, versions, *entries, *itersPoint, *itersJoin)
	fmt.Print(md)
	if *out == "" {
		return
	}
	if err := harness.WriteResults(*out, md); err != nil {
		log.Fatal(err)
	}
	log.Printf("results written to %s", *out)
}

func run(ctx context.Context, db *sql.DB, backend string, d design, accts []account, moves []movement, batch, itersPoint, itersJoin int, seed int64) (result, error) {
	r := result{backend: backend, design: d.name, accounts: len(accts), rows: len(moves)}
	var err error
	if r.load, err = load(ctx, db, d, accts, moves, batch); err != nil {
		return r, err
	}
	if backend == "postgres" {
		db.ExecContext(ctx, "ANALYZE "+d.movements())
		db.ExecContext(ctx, "ANALYZE "+d.accounts())
	}
	r.table, r.index = sizes(ctx, db, backend, d)
	rng := rand.New(rand.NewSource(seed))
	var ds []time.Duration
	for i := 0; i < itersPoint; i++ {
		a := accts[rng.Intn(len(accts))]
		t := time.Now()
		if _, _, err := byAccount(ctx, db, d, a.id); err != nil {
			return r, err
		}
		ds = append(ds, time.Since(t))
	}
	r.byAccount = harness.Summarise(ds)
	ds = ds[:0]
	for i := 0; i < itersPoint; i++ {
		m := moves[rng.Intn(len(moves))]
		t := time.Now()
		if _, err := byID(ctx, db, d, m.id); err != nil {
			return r, err
		}
		ds = append(ds, time.Since(t))
	}
	r.byID = harness.Summarise(ds)
	ds = ds[:0]
	for i := 0; i < itersJoin; i++ {
		t := time.Now()
		n, err := join(ctx, db, d)
		if err != nil {
			return r, err
		}
		if int(n) != len(moves) {
			return r, fmt.Errorf("join matched %d of %d movements", n, len(moves))
		}
		ds = append(ds, time.Since(t))
	}
	r.jn = harness.Summarise(ds)
	return r, nil
}

func render(rs []result, skipped []string, versions map[string]string, entries, itersPoint, itersJoin int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**Run:** %s · %s/%s · %s · %d movements per account · point queries ×%d · join ×%d\n\n",
		time.Now().Format("2006-01-02 15:04"), runtime.GOOS, runtime.GOARCH, runtime.Version(), entries, itersPoint, itersJoin)
	order := []string{"pglike-file", "pglike-memory", "postgres"}
	for _, name := range order {
		if v, ok := versions[name]; ok {
			fmt.Fprintf(&b, "- %s: %s\n", name, v)
		}
	}
	for _, s := range skipped {
		fmt.Fprintf(&b, "- skipped %s\n", s)
	}
	b.WriteString("\n")

	var scales []int
	seen := map[int]bool{}
	for _, r := range rs {
		if !seen[r.accounts] {
			seen[r.accounts] = true
			scales = append(scales, r.accounts)
		}
	}
	for _, n := range scales {
		fmt.Fprintf(&b, "### %s accounts (%s movements)\n\n", harness.FmtInt(n), harness.FmtInt(n*entries))
		b.WriteString("| Backend | Design | Load | Table | Indexes | By account p50 | By account p99 | By id p50 | By id p99 | Join p50 |\n")
		b.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
		for _, r := range rs {
			if r.accounts != n {
				continue
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
				r.backend, r.design, harness.FmtDur(r.load), fmtBytes(r.table), fmtBytes(r.index),
				harness.FmtDur(r.byAccount.P50), harness.FmtDur(r.byAccount.P99),
				harness.FmtDur(r.byID.P50), harness.FmtDur(r.byID.P99), harness.FmtDur(r.jn.P50))
		}
		b.WriteString("\n")
	}
	b.WriteString("On pglike-file, Table is the whole database file (both tables and their indexes) and Indexes is not separated.\n\n")

	b.WriteString("### TEXT relative to UUID (p50)\n\n")
	b.WriteString("| Backend | Accounts | Design | Table | Indexes | By account | By id | Join |\n")
	b.WriteString("|---|---:|---|---:|---:|---:|---:|---:|\n")
	for _, name := range order {
		for _, n := range scales {
			var base *result
			for i := range rs {
				if rs[i].backend == name && rs[i].accounts == n && rs[i].design == "uuid" {
					base = &rs[i]
				}
			}
			if base == nil {
				continue
			}
			for i := range rs {
				r := &rs[i]
				if r.backend != name || r.accounts != n || r.design == "uuid" {
					continue
				}
				fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n", name, harness.FmtInt(n), r.design,
					ratioInt(r.table, base.table), ratioInt(r.index, base.index),
					ratio(r.byAccount.P50, base.byAccount.P50), ratio(r.byID.P50, base.byID.P50), ratio(r.jn.P50, base.jn.P50))
			}
		}
	}
	return b.String()
}

func ratio(a, b time.Duration) string {
	if b == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2fx", float64(a)/float64(b))
}

func ratioInt(a, b int64) string {
	if b == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2fx", float64(a)/float64(b))
}

func fmtBytes(n int64) string {
	switch {
	case n == 0:
		return "-"
	case n < 1<<20:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	case n < 1<<30:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	}
}
