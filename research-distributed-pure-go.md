# Distributed go-postgres: A Pure-Go Research Proposal

A research proposal for turning go-postgres into a distributed, replicated, browser-capable database while keeping the no-cgo guarantee that makes the project useful for gokrazy and WASM targets.

**Date**: 2026-05-02
**Status**: Proposal — no code yet
**Constraint**: Pure Go end to end. No cgo on any node. No native SQLite.

## Motivation

go-postgres today is a single-process PG-compat layer over `ncruces/go-sqlite3`. The translation pipeline and `pgfuncs.go` give applications a familiar Postgres dialect without the operational weight of a real Postgres server — and without cgo.

The next step is resilience and reach:

1. **Resilience** — survive node loss, allow rolling restarts, replicate writes.
2. **Reach** — run a node inside a browser tab so apps can read locally, work offline, and sync when online.

The natural reference is rqlite. The natural problem is that rqlite depends on `mattn/go-sqlite3` (cgo), which defeats the whole reason for being on `ncruces/go-sqlite3`.

## Proposal

Build a small `internal/cluster` package that combines pure-Go Raft with the existing translate pipeline and ncruces SQLite. Treat rqlite as a design reference, not a dependency.

### Stack

| Concern | Component | Pure Go? |
|---|---|---|
| Consensus | `hashicorp/raft` | yes |
| Raft log store | `hashicorp/raft-boltdb` (bbolt) | yes |
| Snapshot store | `raft.FileSnapshotStore` | yes |
| Transport (servers) | `raft.NetworkTransport` over TCP | yes |
| Transport (browser) | custom `WebSocketTransport` | yes |
| State machine | ncruces/go-sqlite3 + translate pipeline | yes |
| Driver surface | existing `pglike` driver, write interception | yes |

### State machine model

Each node runs a full go-postgres instance. The Raft FSM applies committed log entries by executing translated SQL against the local SQLite. Reads hit the local SQLite directly (stale by default; leader-forwarded for linearisable reads).

### The three real design problems

#### 1. Determinism

Postgres functions like `gen_random_uuid()`, `now()`, `random()` are non-deterministic. If a Raft entry contains the raw SQL, every follower computes a different UUID, and the cluster diverges within one write.

**Resolution.** Add a pre-apply pass to the translate pipeline that runs on the leader only. It substitutes non-deterministic function calls with computed literals before the entry enters the log. Followers then apply pure SQL with no side-channel state.

This pass is the riskiest and most interesting piece of the project. It needs to handle:

- Standalone calls: `INSERT INTO t (id) VALUES (gen_random_uuid())`
- Multiple calls in one statement (each gets its own value)
- Default expressions in DDL (resolved at row-insert time, not DDL apply time — so DDL is unchanged, but `INSERT` paths must materialise defaults)
- Deterministic functions left alone

The existing token-based pipeline is well suited to this — add it as pass 10.

#### 2. Transactions

A Raft entry applies atomically. Interactive transactions with mid-flight round-trips (`BEGIN`, app logic, `COMMIT`) cannot be supported as-is, because the leader cannot hold a transaction open across log entries on followers.

**Resolution.** Buffer the transaction client-side in the driver shim and ship the whole `BEGIN…COMMIT` block as one Raft entry. This covers the common go-postgres use cases (batch inserts, multi-statement migrations) and gives up only the interactive-transaction pattern, which is rarely the right shape for a replicated store anyway.

#### 3. Snapshots

ncruces/go-sqlite3 exposes the SQLite online backup API. `FSMSnapshot.Persist` calls backup to a file; `FSM.Restore` swaps the file in and reopens the connection. No cgo path, no special tooling.

### Driver shim

The existing `pglike` driver wraps a SQLite connection. The cluster shim wraps `pglike`:

- `Exec` / `ExecContext` — run determinism pass, propose to Raft, wait for apply, return result.
- `Query` / `QueryContext` — local read by default; option for leader-forwarded read-index reads.
- Non-leader writes return a redirect error with the leader address; the shim retries transparently.

### Estimated size

| Piece | Lines (rough) |
|---|---:|
| FSM (Apply, Snapshot, Restore) | 300 |
| Determinism pass | 400 |
| Driver shim (write interception, leader forward) | 300 |
| Join / membership API (HTTP) | 200 |
| WebSocket transport (for browser nodes) | 200 |
| Tests | 500 |
| **Total** | **~1900** |

The translate pipeline itself is untouched.

## Browser nodes

The same Go code, compiled with `GOOS=js GOARCH=wasm`, can run inside a browser. ncruces/go-sqlite3 already supports `GOOS=js` with an OPFS-backed VFS, so the FSM side works as-is. Three changes are needed.

### Transport

Browsers cannot open raw TCP. Implement `raft.Transport` over WebSockets. The interface is small and clean — request/response RPC over a persistent connection. Server-side accepts WebSocket upgrades alongside the existing TCP transport.

### Log and stable store

bbolt assumes a filesystem. In the browser, back the Raft log with either an OPFS file (bbolt over OPFS via a small VFS shim) or a separate SQLite table managed by ncruces. The latter is simpler — one OPFS database file holds both the FSM data and the Raft log in different tables.

### Membership

A browser tab is not a reliable voter. Tabs close, networks flap, OPFS storage gets evicted. Making it a voting peer destroys quorum.

**Resolution.** Browser nodes join as **non-voting learners**. They tail the log from any voter, apply entries to their local SQLite, serve local reads, and forward writes to the leader. This matches the rqlite read-only-node pattern and maps cleanly onto offline-first apps: local reads always work, writes block or queue when offline, replay on reconnect.

### Service worker vs SharedWorker

A common confusion. Service workers exist to intercept `fetch` events and are aggressively killed by the browser — wrong host for a long-lived stateful Raft learner.

**Resolution.** Run the learner in a **SharedWorker** (or dedicated Worker if SharedWorker is unavailable). All tabs share one SQLite, one Raft connection, one OPFS handle. The service worker becomes a thin fetch-interceptor that forwards app HTTP requests to the SharedWorker via `postMessage` and returns synthesised responses. The result: the app sees a normal HTTP API; under the hood, requests are answered from a local replicated SQLite.

## What this gives applications

- **Server side**: a 3- or 5-node cluster that survives node loss, with familiar Postgres SQL and no cgo. Fits gokrazy, fits constrained environments.
- **Browser side**: offline-first apps where reads are local and instant, writes sync through the cluster, and the same Go code runs in both places.
- **Migration story**: existing go-postgres users add the cluster shim with no SQL changes.

## What it gives up

- Interactive transactions (multi-round-trip `BEGIN…COMMIT`).
- Hard linearisability without explicit opt-in (default reads are stale-local).
- Single-process simplicity. Operating a Raft cluster is real work, even with hashicorp/raft.

## Open questions

1. **Determinism coverage.** Can the pre-apply pass reliably catch every non-deterministic call site, including ones inside CTEs, subqueries, and `INSERT … SELECT`? Needs an enumerated list of PG functions and a fuzz-test harness.
2. **Schema changes.** DDL on the leader vs followers — same SQL, but `CREATE INDEX` timing under load matters. Probably fine, needs verification.
3. **Browser quotas.** OPFS storage limits vary. What is the eviction story for a learner that has fallen behind by more than its local snapshot can hold?
4. **WebSocket transport correctness.** hashicorp/raft has subtle expectations about pipelining and ordering. The custom transport needs careful testing against the project's own test suite.
5. **Backup / point-in-time.** Does the cluster expose a `pg_dump`-equivalent? Probably yes, by streaming the FSM snapshot.

## Suggested next steps

1. Spike the determinism pass against the existing translate pipeline. Lowest cost, highest risk — best to learn early.
2. Build the FSM and a 3-node in-process cluster using TCP transport. Validate snapshot/restore.
3. Add the driver shim and run the existing test suite against a clustered backend.
4. Only then take on the WebSocket transport and browser learner.

The first three steps are a self-contained server-side feature. The browser work is a clean follow-on, not a prerequisite.

## Why this is worth doing

There is no pure-Go, PG-compatible, browser-capable, replicated database. rqlite is close but cgo. CockroachDB is pure Go but heavyweight and not browser-targetable. SQLite-based replication tools (Litestream, LiteFS) replicate but don't give consensus. The combination of `ncruces/go-sqlite3` plus go-postgres plus `hashicorp/raft` is a genuinely new point in the design space, and the pieces all already exist.
