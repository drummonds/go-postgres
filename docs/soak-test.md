# Soak Test

The soak test (`TestSoak`) runs a sustained mixed workload against the pglike driver to detect memory leaks, performance degradation, and correctness issues over time.

## Workload Mix

Each worker picks an operation at random per iteration:

| Operation       | Weight | Description                                         |
|-----------------|--------|-----------------------------------------------------|
| Insert user     | 30%    | INSERT into `soak_users` with random name/score     |
| Insert event    | 20%    | INSERT into `soak_events` linked to a random user   |
| Read query      | 30%    | One of: COUNT, filtered SELECT, JOIN, gen_random_uuid |
| Update          | 10%    | UPDATE score on a random user                       |
| Transaction     | 10%    | BEGIN → INSERT user → INSERT event → COMMIT         |

50% of operations are writes, so the dataset grows continuously throughout the test.

## Schema

Two tables: `soak_users` (id, name, email, active, score, created_at) and `soak_events` (id, user_id, kind, payload, created_at) with a foreign key relationship.

## Metrics

Emitted as JSONL at a configurable interval (`SOAK_METRIC_INTERVAL`):

- **ops_per_sec** — throughput in the most recent interval (not cumulative)
- **cum_ops_per_sec** — cumulative throughput since test start
- **heap_alloc_mb / heap_sys_mb** — Go heap metrics
- **num_gc** — garbage collection count
- **avg_latency_us** — cumulative average latency per operation
- **total_ops / total_errors** — running totals

## Expected Performance Characteristics

**Throughput declines over time.** This is expected and not a bug:

1. **Growing dataset** — 50% of operations are inserts. After an hour with 4 workers, there may be hundreds of thousands of rows.
2. **Scan-heavy queries** — `ORDER BY RANDOM() LIMIT 1` requires a full table scan. JOIN queries also scan more rows as tables grow.
3. **SQLite single-writer** — all writes are serialized through SQLite's write lock, so write throughput is bounded regardless of worker count.

The interval ops/sec metric shows the real-time throughput. The cumulative ops/sec is a smoothed average that will always trend toward the steady-state rate.

A healthy soak run shows:
- Declining but stabilising ops/sec (not crashing to zero)
- Bounded heap allocation (no unbounded memory growth)
- Zero errors

## Configuration

| Env var                | Default | Description                    |
|------------------------|---------|--------------------------------|
| `SOAK_DURATION`        | 1m      | Test duration (e.g. `1h`)      |
| `SOAK_WORKERS`         | 4       | Concurrent goroutines          |
| `SOAK_METRIC_INTERVAL` | 5s      | How often to emit metrics      |
| `SOAK_OUTPUT`          | stdout  | JSONL output file path         |

## Generating a Chart

```bash
# Run locally
SOAK_DURATION=5m SOAK_OUTPUT=soak.jsonl go test -run TestSoak -timeout 10m -count=1

# Plot
go run ./cmd/soak-plot < soak.jsonl > docs/soak-chart.svg
```

The chart shows interval ops/sec (solid blue) and cumulative ops/sec (dashed red) over time.
