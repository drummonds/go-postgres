# Soak Testing and Performance Research

Results from stability testing and benchmarking of go-postgres, including memory behaviour under sustained load and the impact of query translation caching.

**Date**: 2026-03-20
**Branch**: task/WTstable
**Platform**: Linux amd64, Intel Core Ultra 7 165H, Go 1.26.0

## Translation Pipeline Benchmarks

Every query passes through tokenization followed by 9 ordered translation passes. These benchmarks measure the cost of that pipeline.

### Full Translation (Tokenize + Translate + Reassemble)

| Query Type | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Simple SELECT with ILIKE, IS TRUE | 62,100 | 112,596 | 191 |
| Complex DDL (8 columns, UUID, TIMESTAMPTZ, etc.) | 132,800 | 234,760 | 290 |
| INSERT with RETURNING | 37,900 | 55,521 | 157 |

### Pipeline Breakdown

| Stage | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Tokenize only | 6,530 | 6,624 | 54 |
| Translate passes only (pre-tokenized) | 54,100 | 105,706 | 132 |
| Full Translate (tokenize + passes + reassemble) | 62,100 | 112,596 | 191 |

**Observations**:
- Tokenization accounts for ~10% of wall time but ~30% of allocations
- The 9 translation passes dominate at ~87% of wall time
- Each pass creates new token slices — this is where allocations accumulate
- Total allocation per query is 110-235 KB depending on complexity

### Query Cache Performance

A FIFO translation cache (keyed by input SQL, max 1000 entries) was implemented to avoid repeated translation of identical queries.

| Mode | ns/op | B/op | allocs/op | Speedup |
|---|---:|---:|---:|---:|
| Uncached | 62,100 | 112,596 | 191 | 1x |
| Cached (hit) | 39 | 0 | 0 | ~1,600x |

The cache eliminates all allocation on hit. For typical application workloads with a fixed set of prepared queries, this effectively reduces translation cost to zero after warmup.

**Implementation**: `translate_cache.go` — `sync.RWMutex`-protected map with FIFO eviction. Thread-safe for connection pool use.

## Native Soak Test Results

The soak test runs a sustained CRUD workload simulating interest calculations: batch INSERT accounts, SELECT all, compute interest, INSERT transactions, UPDATE balances, aggregate queries, and periodic DELETE of oldest transactions.

### Configuration

- Database: file-backed pglike (SQLite via ncruces/go-sqlite3)
- Batch size: 10 accounts per iteration
- Delete threshold: 5000 transactions (prune oldest 10% when exceeded)
- Memory limit: 512 MB

### 10-Second Run

| Metric | Value |
|---|---|
| Final Alloc | 802 KB |
| Total Alloc | 127.70 MB |
| Heap Objects | 2,266 |
| GC Cycles | 44 |

### 30-Second Run

| Metric | Value |
|---|---|
| Final Alloc | 2.23 MB |
| Total Alloc | 411.84 MB |
| Heap Objects | 6,587 |
| GC Cycles | 147 |

**Observations**:
- Memory is well-controlled — GC keeps live heap under 3 MB even as total allocation grows
- TotalAlloc grows linearly (~14 MB/s) reflecting the allocation-heavy translation pipeline
- With query caching enabled, TotalAlloc growth would drop dramatically since most allocations come from repeated translation
- No memory leaks detected — final alloc remains stable relative to data volume

## Architecture

### Packages

```
memcheck/           Memory monitoring (reusable, importable by gobank)
  memcheck.go       Stats, Monitor, FormatBytes
  memcheck_wasm.go  WASM init: debug.SetMemoryLimit(900MB)

soakwork/           Workload engine
  workload.go       Config, Workload, Report, Setup, RunIteration, Run

cmd/soakwasm/       Browser WASM soak test
  main_wasm.go      JS-exported functions for browser control
  index.html        UI with discovery/sustained modes

cmd/soakserve/      HTTP server for WASM soak test
  main.go           Serves cmd/soakwasm/ on :8080
```

### Test Modes

| Mode | Command | Purpose |
|---|---|---|
| Quick smoke | `go test ./soakwork/...` | Verify workload runs 5 iterations |
| Native soak | `task test:soak` | 5 min sustained load, file-backed DB |
| Long soak | `task test:soak:long` | 1 hour sustained load |
| Benchmarks | `task bench` | Translation pipeline performance |
| WASM browser | `task soak:wasm` | Manual browser testing |
| Container | `task container:run` | 512MB cgroup-limited, 1 hour |
| Container OOM | `task container:run:oom` | 128MB limit, stress test |

### WASM Soak Test Modes

**Discovery**: No deletion threshold. Run until the browser OOM-kills the WASM module. The polling UI captures the last-known memory stats before crash. Use this to find the memory ceiling for a given browser.

**Sustained**: Set deletion threshold (default 5000 transactions). The workload prunes oldest 10% when the threshold is exceeded, maintaining a steady-state row count. The test should run indefinitely below the discovered ceiling.

## Recommendations

1. **Enable query caching in the driver**: The 1,600x speedup with zero allocations makes this a clear win. Most applications use a fixed set of queries. Integration point: `conn.Prepare()` in `driver.go` should call `TranslateCached()` instead of `Translate()`.

2. **WASM memory ceiling**: Browser testing is needed to determine the practical limit. The `memcheck` package sets `debug.SetMemoryLimit(900MB)` but actual browser limits vary. Discovery mode will reveal the real ceiling.

3. **Reduce translation allocations**: The 9-pass pipeline allocates 132 token slices per query. A future optimisation could translate in-place on a single mutable slice, but the cache makes this low priority.

4. **Container testing for CI**: The Containerfile produces a static binary in a distroless image. This can be integrated into CI for automated soak testing on every release.

## Related Issues

- [go-postgres#6](https://codeberg.org/hum3/go-postgres/issues/6) — Stability/soak testing infrastructure (umbrella)
- [gobank#6](https://codeberg.org/hum3/gobank/issues/6) — Refactor memory checker to use go-postgres/memcheck
