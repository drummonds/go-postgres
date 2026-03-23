//go:build js && wasm

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"syscall/js"

	_ "codeberg.org/hum3/go-postgres"
	"codeberg.org/hum3/go-postgres/memcheck"
	"codeberg.org/hum3/go-postgres/soakwork"
)

var (
	db      *sql.DB
	monitor *memcheck.Monitor
	wl      *soakwork.Workload
)

func main() {
	monitor = memcheck.NewMonitor(0) // no limit — discovery mode finds the ceiling

	js.Global().Set("goSoakSetup", js.FuncOf(goSoakSetup))
	js.Global().Set("goSoakStep", js.FuncOf(goSoakStep))
	js.Global().Set("goSoakMemStats", js.FuncOf(goSoakMemStats))
	js.Global().Set("goSoakSetDeleteThreshold", js.FuncOf(goSoakSetDeleteThreshold))

	fmt.Println("go-postgres soak WASM module loaded")
	select {}
}

func goSoakSetup(_ js.Value, _ []js.Value) any {
	var err error
	db, err = sql.Open("pglike", ":memory:")
	if err != nil {
		return errJSON(err)
	}
	wl = soakwork.New(db, soakwork.Config{BatchSize: 10}, monitor)
	if err := wl.Setup(); err != nil {
		return errJSON(err)
	}
	return `{"ok":true}`
}

// goSoakStep runs one iteration and returns a JSON report.
// Called from JS setInterval — keeps the browser responsive.
func goSoakStep(_ js.Value, _ []js.Value) any {
	if wl == nil {
		return errJSON(fmt.Errorf("call goSoakSetup first"))
	}
	r, err := wl.RunIteration()
	if err != nil {
		return errJSON(err)
	}
	return reportJSON(r)
}

func goSoakMemStats(_ js.Value, _ []js.Value) any {
	s := memcheck.ReadStats()
	b, _ := json.Marshal(map[string]any{
		"alloc":       s.Alloc,
		"allocHuman":  memcheck.FormatBytes(s.Alloc),
		"totalAlloc":  s.TotalAlloc,
		"sys":         s.Sys,
		"heapObjects": s.HeapObjects,
		"numGC":       s.NumGC,
		"goroutines":  s.Goroutines,
	})
	return string(b)
}

func goSoakSetDeleteThreshold(_ js.Value, args []js.Value) any {
	if len(args) == 0 {
		return errJSON(fmt.Errorf("missing threshold argument"))
	}
	threshold := args[0].Int()
	wl = soakwork.New(db, soakwork.Config{
		BatchSize:       10,
		DeleteThreshold: threshold,
	}, monitor)
	return `{"ok":true}`
}

func reportJSON(r soakwork.Report) string {
	b, _ := json.Marshal(map[string]any{
		"iteration":    r.Iteration,
		"accountCount": r.AccountCount,
		"txCount":      r.TxCount,
		"opsPerSec":    r.OpsPerSec,
		"elapsedSec":   r.Elapsed.Seconds(),
		"alloc":        r.Stats.Alloc,
		"allocHuman":   memcheck.FormatBytes(r.Stats.Alloc),
		"totalAlloc":   r.Stats.TotalAlloc,
		"sys":          r.Stats.Sys,
		"heapObjects":  r.Stats.HeapObjects,
		"numGC":        r.Stats.NumGC,
		"goroutines":   r.Stats.Goroutines,
	})
	return string(b)
}

func errJSON(err error) string {
	b, _ := json.Marshal(map[string]string{"error": err.Error()})
	return string(b)
}
