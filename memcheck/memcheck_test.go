package memcheck

import "testing"

func TestReadStats(t *testing.T) {
	s := ReadStats()
	if s.Alloc == 0 {
		t.Fatal("expected non-zero Alloc")
	}
	if s.Goroutines == 0 {
		t.Fatal("expected non-zero Goroutines")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input uint64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
	}
	for _, tt := range tests {
		got := FormatBytes(tt.input)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMonitorHighLimit(t *testing.T) {
	m := NewMonitor(1 << 40) // 1 TB — won't be exceeded
	exceeded, stats := m.Check()
	if exceeded {
		t.Fatalf("expected not exceeded, Alloc=%d", stats.Alloc)
	}
	if m.Exceeded() {
		t.Fatal("expected Exceeded() == false")
	}
}

func TestMonitorZeroLimit(t *testing.T) {
	m := NewMonitor(1) // 1 byte — always exceeded
	exceeded, _ := m.Check()
	if !exceeded {
		t.Fatal("expected exceeded with 1-byte limit")
	}
	if !m.Exceeded() {
		t.Fatal("expected Exceeded() == true")
	}
	m.Reset()
	if m.Exceeded() {
		t.Fatal("expected Exceeded() == false after Reset()")
	}
}
