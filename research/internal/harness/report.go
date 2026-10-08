package harness

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	markStart = "<!-- results:start -->"
	markEnd   = "<!-- results:end -->"
)

// WriteResults splices results into the document at path.
func WriteResults(path, results string) error {
	doc, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	spliced, err := SpliceResults(string(doc), results)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(spliced), 0o644)
}

// SpliceResults replaces the text between the result markers in doc.
func SpliceResults(doc, results string) (string, error) {
	i := strings.Index(doc, markStart)
	j := strings.Index(doc, markEnd)
	if i < 0 || j < 0 || j < i {
		return "", errors.New("result markers not found in document")
	}
	return doc[:i+len(markStart)] + "\n" + results + doc[j:], nil
}

type Stats struct {
	Mean, P50, P99, Min, Max time.Duration
	N                        int
}

func Summarise(ds []time.Duration) Stats {
	if len(ds) == 0 {
		return Stats{}
	}
	s := make([]time.Duration, len(ds))
	copy(s, ds)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	var total time.Duration
	for _, d := range s {
		total += d
	}
	pct := func(p float64) time.Duration {
		k := int(p*float64(len(s))+0.5) - 1
		if k < 0 {
			k = 0
		}
		if k >= len(s) {
			k = len(s) - 1
		}
		return s[k]
	}
	return Stats{Mean: total / time.Duration(len(s)), P50: pct(0.5), P99: pct(0.99), Min: s[0], Max: s[len(s)-1], N: len(s)}
}

func FmtDur(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%.1fus", float64(d)/float64(time.Microsecond))
	case d < time.Second:
		return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

func FmtInt(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "_" + s[i:]
	}
	return s
}
