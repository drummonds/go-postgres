package memcheck

import (
	"fmt"
	"runtime"
	"sync/atomic"
)

// Stats holds a snapshot of memory and runtime statistics.
type Stats struct {
	Alloc       uint64 // bytes currently allocated on heap
	TotalAlloc  uint64 // cumulative bytes allocated
	Sys         uint64 // bytes obtained from OS
	HeapObjects uint64 // number of allocated heap objects
	NumGC       uint32 // number of completed GC cycles
	Goroutines  int    // number of goroutines
}

// ReadStats returns a snapshot of current memory and runtime statistics.
func ReadStats() Stats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return Stats{
		Alloc:       m.Alloc,
		TotalAlloc:  m.TotalAlloc,
		Sys:         m.Sys,
		HeapObjects: m.HeapObjects,
		NumGC:       m.NumGC,
		Goroutines:  runtime.NumGoroutine(),
	}
}

// FormatBytes formats a byte count as a human-readable string.
func FormatBytes(b uint64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// Monitor tracks memory usage against a configured limit.
type Monitor struct {
	LimitBytes uint64
	exceeded   atomic.Bool
}

// NewMonitor creates a Monitor with the given memory limit in bytes.
func NewMonitor(limitBytes uint64) *Monitor {
	return &Monitor{LimitBytes: limitBytes}
}

// Check reads current stats and sets the exceeded flag if Alloc >= LimitBytes.
// Returns whether the limit was exceeded and the current stats.
func (m *Monitor) Check() (bool, Stats) {
	s := ReadStats()
	if m.LimitBytes > 0 && s.Alloc >= m.LimitBytes {
		m.exceeded.Store(true)
		return true, s
	}
	return m.exceeded.Load(), s
}

// Exceeded returns whether the memory limit has been exceeded.
func (m *Monitor) Exceeded() bool {
	return m.exceeded.Load()
}

// Reset clears the exceeded flag.
func (m *Monitor) Reset() {
	m.exceeded.Store(false)
}
