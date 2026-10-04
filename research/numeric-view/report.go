package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	markStart = "<!-- results:start -->"
	markEnd   = "<!-- results:end -->"
)

// spliceResults replaces the text between the result markers in doc.
func spliceResults(doc, results string) (string, error) {
	i := strings.Index(doc, markStart)
	j := strings.Index(doc, markEnd)
	if i < 0 || j < 0 || j < i {
		return "", errors.New("result markers not found in document")
	}
	return doc[:i+len(markStart)] + "\n" + results + doc[j:], nil
}

type stats struct {
	mean, p50, p99, min, max time.Duration
	n                        int
}

func summarise(ds []time.Duration) stats {
	if len(ds) == 0 {
		return stats{}
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
	return stats{mean: total / time.Duration(len(s)), p50: pct(0.5), p99: pct(0.99), min: s[0], max: s[len(s)-1], n: len(s)}
}

func fmtDur(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%.1fus", float64(d)/float64(time.Microsecond))
	case d < time.Second:
		return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

func fmtInt(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "_" + s[i:]
	}
	return s
}
