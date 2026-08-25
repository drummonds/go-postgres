package pglike

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The conformance corpus in testdata/corpus/ is the language-neutral spec
// shared with py-postgres. Format and cell-rendering rules are documented
// in testdata/corpus/README.md.

type corpusCase struct {
	name        string
	line        int
	skip        string
	setup       string
	query       string
	params      []any
	expect      [][]string
	expectMatch [][]string
	expectErr   string
	hasExpect   bool
}

func TestCorpus(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.sql"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no corpus files found")
	}
	for _, file := range files {
		cases, err := parseCorpusFile(file)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		base := strings.TrimSuffix(filepath.Base(file), ".sql")
		t.Run(base, func(t *testing.T) {
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					runCorpusCase(t, file, c)
				})
			}
		})
	}
}

func parseCorpusFile(path string) ([]corpusCase, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cases []corpusCase
	var cur *corpusCase
	section := ""
	var setup, query []string
	lineNo := 0

	flush := func() error {
		if cur == nil {
			return nil
		}
		cur.setup = strings.TrimSpace(strings.Join(setup, "\n"))
		cur.query = strings.TrimSpace(strings.Join(query, "\n"))
		if cur.query == "" {
			return fmt.Errorf("line %d: case %q has no query", cur.line, cur.name)
		}
		n := 0
		if cur.hasExpect {
			n++
		}
		if cur.expectMatch != nil {
			n++
		}
		if cur.expectErr != "" {
			n++
		}
		if n != 1 {
			return fmt.Errorf("line %d: case %q needs exactly one of expect/expect-match/expect-error", cur.line, cur.name)
		}
		cases = append(cases, *cur)
		return nil
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if directive, arg, ok := corpusDirective(trimmed); ok {
			switch directive {
			case "case":
				if err := flush(); err != nil {
					return nil, err
				}
				cur = &corpusCase{name: arg, line: lineNo}
				section, setup, query = "", nil, nil
			case "skip":
				cur.skip = arg
			case "setup":
				section = "setup"
			case "query":
				section = "query"
			case "params":
				cur.params = parseCorpusParams(arg)
			case "expect":
				cur.hasExpect = true
				cur.expect = [][]string{}
				section = "expect"
			case "expect-match":
				cur.expectMatch = [][]string{}
				section = "expect-match"
			case "expect-error":
				cur.expectErr = arg
				section = ""
			default:
				return nil, fmt.Errorf("line %d: unknown directive %q", lineNo, directive)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "--") {
			continue // comment
		}
		if trimmed == "" && section != "setup" && section != "query" {
			continue
		}
		switch section {
		case "setup":
			setup = append(setup, line)
		case "query":
			query = append(query, line)
		case "expect":
			cur.expect = append(cur.expect, splitCorpusRow(trimmed))
		case "expect-match":
			cur.expectMatch = append(cur.expectMatch, splitCorpusRow(trimmed))
		default:
			if trimmed != "" {
				return nil, fmt.Errorf("line %d: SQL outside any section: %q", lineNo, trimmed)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return cases, nil
}

// corpusDirective reports whether a line is a "-- name:" or "-- name: arg"
// directive. Only known directive names match, so ordinary SQL comments in
// fixture files stay comments.
func corpusDirective(line string) (name, arg string, ok bool) {
	rest, found := strings.CutPrefix(line, "--")
	if !found {
		return "", "", false
	}
	rest = strings.TrimSpace(rest)
	name, arg, found = strings.Cut(rest, ":")
	if !found {
		return "", "", false
	}
	name = strings.TrimSpace(name)
	switch name {
	case "case", "skip", "setup", "query", "params", "expect", "expect-match", "expect-error":
		return name, strings.TrimSpace(arg), true
	}
	return "", "", false
}

func splitCorpusRow(line string) []string {
	cells := strings.Split(line, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

func parseCorpusParams(arg string) []any {
	if arg == "" {
		return nil
	}
	var params []any
	for _, cell := range splitCorpusRow(arg) {
		switch cell {
		case "NULL":
			params = append(params, nil)
		default:
			if i, err := strconv.ParseInt(cell, 10, 64); err == nil {
				params = append(params, i)
			} else if f, err := strconv.ParseFloat(cell, 64); err == nil {
				params = append(params, f)
			} else {
				params = append(params, cell)
			}
		}
	}
	return params
}

func runCorpusCase(t *testing.T, file string, c corpusCase) {
	if c.skip != "" {
		t.Skipf("skipped: %s", c.skip)
	}
	db := openTestDB(t)
	if c.setup != "" {
		if _, err := db.Exec(c.setup); err != nil {
			t.Fatalf("%s: setup failed: %v", file, err)
		}
	}

	if c.expectErr != "" {
		_, err := db.Exec(c.query, c.params...)
		if err == nil {
			t.Fatalf("query succeeded, want SQLSTATE %s", c.expectErr)
		}
		var pgErr *PGError
		if !errors.As(err, &pgErr) {
			t.Fatalf("got %T (%v), want *PGError", err, err)
		}
		if pgErr.Code != c.expectErr {
			t.Errorf("SQLSTATE = %s, want %s (%v)", pgErr.Code, c.expectErr, err)
		}
		return
	}

	rows, err := db.Query(c.query, c.params...)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	var got [][]string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("scan: %v", err)
		}
		row := make([]string, len(cols))
		for i, v := range vals {
			row[i] = renderCorpusCell(v)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	want := c.expect
	if c.expectMatch != nil {
		want = c.expectMatch
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d\n  got: %v", len(got), len(want), got)
	}
	for r, wantRow := range want {
		if len(got[r]) != len(wantRow) {
			t.Fatalf("row %d: got %d cells, want %d\n  got: %v", r, len(got[r]), len(wantRow), got[r])
		}
		for i, wantCell := range wantRow {
			gotCell := got[r][i]
			if c.expectMatch != nil {
				re, err := regexp.Compile("^(?:" + wantCell + ")$")
				if err != nil {
					t.Fatalf("row %d cell %d: bad pattern %q: %v", r, i, wantCell, err)
				}
				if !re.MatchString(gotCell) {
					t.Errorf("row %d cell %d: %q does not match %q", r, i, gotCell, wantCell)
				}
			} else if gotCell != wantCell {
				t.Errorf("row %d cell %d: got %q, want %q", r, i, gotCell, wantCell)
			}
		}
	}
}

// renderCorpusCell converts a scanned value to the canonical cell text
// defined in testdata/corpus/README.md.
func renderCorpusCell(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case []byte:
		return string(x)
	case string:
		return x
	case time.Time:
		return x.UTC().Format("2006-01-02 15:04:05")
	default:
		return fmt.Sprint(x)
	}
}
