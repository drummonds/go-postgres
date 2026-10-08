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
	"runtime"
	"strconv"
	"strings"
	"time"

	"git.bytestone.uk/hum3/go-postgres/research/internal/harness"
)

// A result is one design on one backend at one scale.
type result struct {
	backend, design string
	accounts        int
	rows            int
	load            time.Duration
	all, one        harness.Stats
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
	for _, b := range harness.Backends(tmp) {
		if !want[b.Name] {
			continue
		}
		db, closeDB, err := b.Open()
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s: %v", b.Name, err))
			log.Printf("skip %s: %v", b.Name, err)
			continue
		}
		versions[b.Name] = harness.ServerVersion(ctx, db)
		for _, n := range scales {
			ms := generate(n, *entries, *seed)
			want := oracle(ms)
			for _, d := range designs {
				log.Printf("%s %s n=%d", b.Name, d.name, n)
				r, err := run(ctx, db, b.Name, d, ms, want, *batch, *itersAll, *itersOne, *seed)
				if err != nil {
					log.Fatalf("%s %s n=%d: %v", b.Name, d.name, n, err)
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
	if err := harness.WriteResults(*out, md); err != nil {
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
	r.all = harness.Summarise(all)
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
	r.one = harness.Summarise(one)
	return r, nil
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
		fmt.Fprintf(&b, "### %s accounts (%s rows)\n\n", harness.FmtInt(n), harness.FmtInt(n*entries))
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
				r.backend, r.design, harness.FmtDur(r.load), harness.FmtDur(r.all.P50), harness.FmtDur(r.all.P99), harness.FmtDur(r.one.P50), harness.FmtDur(r.one.P99), exact)
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
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", name, harness.FmtInt(n),
				ratio(ie.all.P50, ctl.all.P50), ratio(ie.one.P50, ctl.one.P50),
				ratio(nn.all.P50, ctl.all.P50), ratio(nn.one.P50, ctl.one.P50))
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
