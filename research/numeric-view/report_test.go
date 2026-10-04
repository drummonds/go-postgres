package main

import (
	"strings"
	"testing"
	"time"
)

func TestSpliceResultsReplacesOnlyTheMarkedRegion(t *testing.T) {
	doc := "# Title\n\nintro\n\n<!-- results:start -->\nold\n<!-- results:end -->\n\n## Verdict\n"
	got, err := spliceResults(doc, "new tables\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "# Title\n\nintro\n\n<!-- results:start -->\nnew tables\n<!-- results:end -->\n\n## Verdict\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if _, err := spliceResults("no markers", "x"); err == nil {
		t.Fatal("missing markers must error")
	}
}

func TestStatsSummariseLatencies(t *testing.T) {
	var ds []time.Duration
	for i := 1; i <= 100; i++ {
		ds = append(ds, time.Duration(i)*time.Millisecond)
	}
	s := summarise(ds)
	if s.mean != 50500*time.Microsecond || s.p50 != 50*time.Millisecond || s.p99 != 99*time.Millisecond {
		t.Fatalf("got %+v", s)
	}
	if strings.TrimSpace(fmtDur(1500*time.Microsecond)) != "1.50ms" || fmtDur(250*time.Microsecond) != "250.0us" {
		t.Fatalf("fmtDur: %q %q", fmtDur(1500*time.Microsecond), fmtDur(250*time.Microsecond))
	}
}
