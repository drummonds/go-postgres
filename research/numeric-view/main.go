// Command numeric-view measures whether a ledger that stores integer
// amounts at a fixed exponent can publish NUMERIC balances through a view
// at acceptable cost, on pglike and on native PostgreSQL. It writes its
// results between the result markers of research-numeric-views.md.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "git.bytestone.uk/hum3/go-postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type backend struct {
	name  string
	open  func() (*sql.DB, func(), error)
	notes string
}

func backends(tmp string) []backend {
	return []backend{
		{name: "pglike-file", open: func() (*sql.DB, func(), error) {
			path := filepath.Join(tmp, "numeric-view.db")
			os.Remove(path)
			db, err := sql.Open("pglike", path)
			return db, func() { db.Close(); os.Remove(path) }, err
		}},
		{name: "pglike-memory", open: func() (*sql.DB, func(), error) {
			db, err := sql.Open("pglike", ":memory:")
			return db, func() { db.Close() }, err
		}},
		{name: "postgres", open: func() (*sql.DB, func(), error) {
			dsn := os.Getenv("BENCH_PG_DSN")
			if dsn == "" {
				return nil, nil, fmt.Errorf("BENCH_PG_DSN not set")
			}
			db, err := sql.Open("pgx", dsn)
			if err != nil {
				return nil, nil, err
			}
			if err := db.Ping(); err != nil {
				db.Close()
				return nil, nil, err
			}
			return db, func() { db.Close() }, nil
		}},
	}
}

// A result is one design on one backend at one scale.
type result struct {
	backend, design string
	accounts        int
	rows            int
	load            time.Duration
	all, one        stats
	exact           bool
	mismatches      int
	example         string
}

func main() {
	scalesFlag := flag.String("scales", "1000,10000,100000", "comma-separated account counts")
	entries := flag.Int("entries", 100, "movements per account")
	batch := flag.Int("batch", 500, "rows per INSERT statement")
	itersAll := flag.Int("iters-all", 5, "iterations of the all-balances query")
	itersOne := flag.Int("iters-one", 200, "iterations of the one-account query")
	backendsFlag := flag.String("backends", "pglike-file,pglike-memory,postgres", "backends to run")
	out := flag.String("out", "../../research-numeric-views.md", "document to splice results into (empty: stdout only)")
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

	tmp, err := os.MkdirTemp("", "numeric-view")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	ctx := context.Background()
	var results []result
	var skipped []string
	versions := map[string]string{}
	for _, b := range backends(tmp) {
		if !want[b.name] {
			continue
		}
		db, closeDB, err := b.open()
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s: %v", b.name, err))
			log.Printf("skip %s: %v", b.name, err)
			continue
		}
		versions[b.name] = serverVersion(ctx, db)
		for _, n := range scales {
			ms := generate(n, *entries, *seed)
			want := oracle(ms)
			for _, d := range designs {
				log.Printf("%s %s n=%d", b.name, d.name, n)
				r, err := run(ctx, db, b.name, d, ms, want, *batch, *itersAll, *itersOne, *seed)
				if err != nil {
					log.Fatalf("%s %s n=%d: %v", b.name, d.name, n, err)
				}
				results = append(results, r)
			}
			for _, d := range designs {
				db.ExecContext(ctx, "DROP VIEW IF EXISTS "+d.view())
				db.ExecContext(ctx, "DROP TABLE IF EXISTS "+d.table())
			}
		}
		closeDB()
	}

	md := render(results, skipped, versions, *entries, *itersAll, *itersOne)
	fmt.Print(md)
	if *out == "" {
		return
	}
	doc, err := os.ReadFile(*out)
	if err != nil {
		log.Fatal(err)
	}
	spliced, err := spliceResults(string(doc), md)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, []byte(spliced), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("results written to %s", *out)
}

func run(ctx context.Context, db *sql.DB, backend string, d design, ms []movement, want map[int64]*big.Int, batch, itersAll, itersOne int, seed int64) (result, error) {
	r := result{backend: backend, design: d.name, accounts: len(want), rows: len(ms)}
	var err error
	if r.load, err = load(ctx, db, d, ms, batch); err != nil {
		return r, err
	}
	var all []time.Duration
	for i := 0; i < itersAll; i++ {
		t := time.Now()
		got, err := allBalances(ctx, db, d)
		if err != nil {
			return r, err
		}
		all = append(all, time.Since(t))
		if i == 0 {
			bad := checkExact(want, got, d.scale)
			r.exact, r.mismatches = len(bad) == 0, len(bad)
			if len(bad) > 0 {
				r.example = bad[0].String()
			}
		}
	}
	r.all = summarise(all)
	rng := rand.New(rand.NewSource(seed))
	var one []time.Duration
	for i := 0; i < itersOne; i++ {
		acct := rng.Int63n(int64(len(want))) + 1
		t := time.Now()
		if _, err := oneBalance(ctx, db, d, acct); err != nil {
			return r, err
		}
		one = append(one, time.Since(t))
	}
	r.one = summarise(one)
	return r, nil
}

func serverVersion(ctx context.Context, db *sql.DB) string {
	var v string
	if err := db.QueryRowContext(ctx, "SELECT version()").Scan(&v); err == nil {
		if i := strings.Index(v, " on "); i > 0 {
			v = v[:i]
		}
		return v
	}
	if err := db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&v); err == nil {
		return "pglike (working tree) over SQLite " + v
	}
	return "unknown"
}

func render(rs []result, skipped []string, versions map[string]string, entries, itersAll, itersOne int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**Run:** %s · %s/%s · %s · %d movements per account · all-balances ×%d · one-account ×%d\n\n",
		time.Now().Format("2006-01-02 15:04"), runtime.GOOS, runtime.GOARCH, runtime.Version(), entries, itersAll, itersOne)
	for _, name := range []string{"pglike-file", "pglike-memory", "postgres"} {
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
		fmt.Fprintf(&b, "### %s accounts (%s rows)\n\n", fmtInt(n), fmtInt(n*entries))
		b.WriteString("| Backend | Design | Load | All balances p50 | All p99 | One account p50 | One p99 | Exact |\n")
		b.WriteString("|---|---|---:|---:|---:|---:|---:|---|\n")
		for _, r := range rs {
			if r.accounts != n {
				continue
			}
			exact := "yes"
			if !r.exact {
				exact = fmt.Sprintf("no (%d of %d)", r.mismatches, r.accounts)
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
				r.backend, r.design, fmtDur(r.load), fmtDur(r.all.p50), fmtDur(r.all.p99), fmtDur(r.one.p50), fmtDur(r.one.p99), exact)
		}
		b.WriteString("\n")
		for _, r := range rs {
			if r.accounts == n && !r.exact {
				fmt.Fprintf(&b, "- %s %s first mismatch: %s\n", r.backend, r.design, r.example)
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("### Overhead relative to the integer control (p50)\n\n")
	b.WriteString("| Backend | Accounts | int-exponent all | int-exponent one | native-numeric all | native-numeric one |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|\n")
	for _, name := range []string{"pglike-file", "pglike-memory", "postgres"} {
		for _, n := range scales {
			var ctl, ie, nn *result
			for i := range rs {
				r := &rs[i]
				if r.backend != name || r.accounts != n {
					continue
				}
				switch r.design {
				case "int-control":
					ctl = r
				case "int-exponent":
					ie = r
				case "native-numeric":
					nn = r
				}
			}
			if ctl == nil || ie == nil || nn == nil {
				continue
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", name, fmtInt(n),
				ratio(ie.all.p50, ctl.all.p50), ratio(ie.one.p50, ctl.one.p50),
				ratio(nn.all.p50, ctl.all.p50), ratio(nn.one.p50, ctl.one.p50))
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
