# NUMERIC Balance Views: integer+exponent storage vs native NUMERIC

Is it practical for a ledger that stores amounts as integers at a fixed
exponent to publish balances as `NUMERIC` with seven decimal places through
a view, on pglike (go-postgres over SQLite) and on native PostgreSQL? And
how does that compare with storing `NUMERIC` directly?

**Status**: complete. Results below are from the run recorded in the results header.
Program: `research/numeric-view/` (own Go module). Run with
`task research:numeric-view`; set `BENCH_PG_DSN` for the native pass.

## Why

go-luca stores every movement as a BIGINT at the commodity's exponent and
gobank's position views (ADR-0002 stage 3) must publish balances as
NUMERIC that read identically on pglike in development and on PostgreSQL
in production. go-postgres v0.7.0 added exact `::numeric` expressions so a
view can do that conversion. This study asks what it costs.

## Vocabulary

| Term | Meaning |
|---|---|
| Movement | One ledger entry: `account_id`, `amount` |
| Balance | `SUM(amount)` per account, published by a view |
| Exponent | Power of ten the integer amount is scaled by; fixed at −7 here, so `1_0000000` is one unit |
| Design | How the amount is stored and how the view converts it |
| Backend | `pglike-file`, `pglike-memory` (`:memory:`), `postgres` (native, via pgx through `database/sql`) |
| Oracle | Exact per-account sum computed in Go with `math/big`; every published balance is checked against it |

## Designs

| Design | `amount` column | View expression | Role |
|---|---|---|---|
| int-control | `BIGINT` | `SUM(amount)` | Baseline: aggregation cost with no conversion |
| int-exponent | `BIGINT` | `round(SUM(amount)::numeric / 10000000, 7)` | The go-luca design under test |
| native-numeric | `NUMERIC(20,7)` | `SUM(amount)` | What you would write for PostgreSQL alone |

Each design has its own table and view, identical on both drivers:

```sql
CREATE TABLE movements_<design> (id BIGINT NOT NULL, account_id BIGINT NOT NULL, amount <column> NOT NULL);
CREATE INDEX movements_<design>_account ON movements_<design> (account_id);
CREATE VIEW balances_<design> AS
  SELECT account_id, <expression> AS balance FROM movements_<design> GROUP BY account_id;
```

## Method

- n accounts × 100 movements each, amounts uniformly random in ±100 units
  with all seven fractional digits in play, fixed seed, interleaved by
  account as a ledger would receive them.
- Load: multi-row `INSERT` (500 rows per statement) through one prepared
  statement in one transaction, the same path on both drivers.
- All balances: `SELECT account_id, balance FROM balances_<design>` (n rows out).
- One account: `SELECT balance FROM balances_<design> WHERE account_id = $1`,
  random accounts, through the view.
- Exact: the first all-balances result is parsed exactly and compared to
  the oracle; a value passes if numerically equal whatever its text form.
- No warm-up; p50 and p99 over the iterations stated in the results header.

## Results

<!-- results:start -->
**Run:** 2026-10-04 21:19 · linux/amd64 · go1.26.4 · 100 movements per account · all-balances ×5 · one-account ×200

- pglike-file: pglike (working tree) over SQLite 3.53.4
- pglike-memory: pglike (working tree) over SQLite 3.53.4
- postgres: PostgreSQL 16.15 (Ubuntu 16.15-0ubuntu0.24.04.1)

### 1_000 accounts (100_000 rows)

| Backend | Design | Load | All balances p50 | All p99 | One account p50 | One p99 | Exact |
|---|---|---:|---:|---:|---:|---:|---|
| pglike-file | int-control | 199.79ms | 44.60ms | 46.94ms | 86.2us | 323.3us | yes |
| pglike-file | int-exponent | 206.20ms | 46.89ms | 51.44ms | 91.3us | 333.0us | yes |
| pglike-file | native-numeric | 221.25ms | 59.76ms | 63.08ms | 136.1us | 592.1us | no (421 of 1000) |
| pglike-memory | int-control | 141.02ms | 46.32ms | 46.75ms | 122.2us | 223.7us | yes |
| pglike-memory | int-exponent | 138.70ms | 53.29ms | 54.12ms | 135.6us | 183.2us | yes |
| pglike-memory | native-numeric | 171.86ms | 57.78ms | 60.59ms | 130.0us | 489.3us | no (421 of 1000) |
| postgres | int-control | 213.10ms | 9.72ms | 11.48ms | 65.7us | 129.8us | yes |
| postgres | int-exponent | 223.58ms | 9.79ms | 12.49ms | 66.5us | 226.7us | yes |
| postgres | native-numeric | 245.54ms | 13.70ms | 16.14ms | 63.8us | 132.7us | yes |

- pglike-file native-numeric first mismatch: account 2: want -500.5420417 got -500.54204169999997
- pglike-memory native-numeric first mismatch: account 2: want -500.5420417 got -500.54204169999997

### 10_000 accounts (1_000_000 rows)

| Backend | Design | Load | All balances p50 | All p99 | One account p50 | One p99 | Exact |
|---|---|---:|---:|---:|---:|---:|---|
| pglike-file | int-control | 2.09s | 534.48ms | 541.33ms | 238.3us | 270.6us | yes |
| pglike-file | int-exponent | 2.10s | 569.21ms | 579.09ms | 249.9us | 759.2us | yes |
| pglike-file | native-numeric | 2.46s | 643.94ms | 670.63ms | 283.9us | 392.8us | no (4347 of 10000) |
| pglike-memory | int-control | 1.80s | 589.77ms | 590.59ms | 293.1us | 518.8us | yes |
| pglike-memory | int-exponent | 1.78s | 659.61ms | 684.62ms | 300.6us | 644.4us | yes |
| pglike-memory | native-numeric | 2.09s | 677.05ms | 709.83ms | 306.2us | 414.1us | no (4347 of 10000) |
| postgres | int-control | 1.94s | 65.13ms | 72.37ms | 114.4us | 322.7us | yes |
| postgres | int-exponent | 2.03s | 58.59ms | 61.56ms | 129.0us | 320.3us | yes |
| postgres | native-numeric | 2.48s | 77.47ms | 81.78ms | 131.4us | 191.8us | yes |

- pglike-file native-numeric first mismatch: account 1: want -144.0563025 got -144.05630250000002
- pglike-memory native-numeric first mismatch: account 1: want -144.0563025 got -144.05630250000002

### 100_000 accounts (10_000_000 rows)

| Backend | Design | Load | All balances p50 | All p99 | One account p50 | One p99 | Exact |
|---|---|---:|---:|---:|---:|---:|---|
| pglike-file | int-control | 22.48s | 5.77s | 5.86s | 289.0us | 364.0us | yes |
| pglike-file | int-exponent | 22.75s | 6.54s | 6.70s | 485.8us | 659.0us | yes |
| pglike-file | native-numeric | 32.88s | 7.31s | 7.67s | 527.3us | 824.8us | no (42786 of 100000) |
| pglike-memory | int-control | 17.17s | 6.09s | 6.14s | 184.0us | 220.3us | yes |
| pglike-memory | int-exponent | 14.55s | 6.07s | 6.13s | 212.2us | 352.3us | yes |
| pglike-memory | native-numeric | 17.50s | 6.61s | 6.65s | 247.9us | 488.9us | no (42786 of 100000) |
| postgres | int-control | 21.16s | 806.99ms | 887.26ms | 525.0us | 878.3us | yes |
| postgres | int-exponent | 19.01s | 765.83ms | 813.42ms | 511.5us | 728.8us | yes |
| postgres | native-numeric | 22.03s | 1.12s | 1.22s | 527.5us | 746.4us | yes |

- pglike-file native-numeric first mismatch: account 1: want -288.2986556 got -288.2986556000001
- pglike-memory native-numeric first mismatch: account 1: want -288.2986556 got -288.2986556000001

### Overhead relative to the integer control (p50)

| Backend | Accounts | int-exponent all | int-exponent one | native-numeric all | native-numeric one |
|---|---:|---:|---:|---:|---:|
| pglike-file | 1_000 | 1.05x | 1.06x | 1.34x | 1.58x |
| pglike-file | 10_000 | 1.06x | 1.05x | 1.20x | 1.19x |
| pglike-file | 100_000 | 1.13x | 1.68x | 1.27x | 1.82x |
| pglike-memory | 1_000 | 1.15x | 1.11x | 1.25x | 1.06x |
| pglike-memory | 10_000 | 1.12x | 1.03x | 1.15x | 1.04x |
| pglike-memory | 100_000 | 1.00x | 1.15x | 1.09x | 1.35x |
| postgres | 1_000 | 1.01x | 1.01x | 1.41x | 0.97x |
| postgres | 10_000 | 0.90x | 1.13x | 1.19x | 1.15x |
| postgres | 100_000 | 0.95x | 0.97x | 1.38x | 1.00x |
<!-- results:end -->

## Analysis

**int-exponent is exact on both drivers.** Every balance matched the
oracle at every scale on pglike (file and `:memory:`) and on PostgreSQL 16.
That is the portability requirement for a view shared between development
and production, and it holds.

**On PostgreSQL the conversion is free.** `SUM(bigint)` already yields a
NUMERIC, so the `::numeric` cast costs nothing and the division and
`round()` run once per output row. int-exponent came out at 0.90–1.01× the
integer control on the all-balances scan and within noise on the indexed
one-account read. Storing `NUMERIC(20,7)` natively is exact but 19–41%
slower on the scan, because every one of the 100 movements per account is
summed in NUMERIC arithmetic rather than in a machine integer.

**On pglike the conversion is cheap.** The view adds one `pg_numeric_*`
call chain (`pg_numeric`, `pg_numeric_div`, `pg_numeric_round` over
`math/big`) per output row, not per movement, so the cost is bounded by
the number of accounts: 1.00–1.15× the control on the all-balances scan.
The `SUM` itself stays SQLite's integer sum, which is where the time goes.
The one-account read was 1.03–1.15× in every cell but one (pglike-file at
100k accounts, 1.68×); the earlier pglike-only run measured 0.99× for that
cell, so it is run-to-run variance in a 300–500µs read, not a systematic
cost.

**native-numeric is wrong on pglike.** A bare `NUMERIC` column is stored
as TEXT and SQLite's `SUM` over text is REAL arithmetic, so about 43% of
balances drifted in the 14th–16th significant digit at every scale
(`-500.5420417` came back as `-500.54204169999997`). It was also the
slowest design on pglike, 9–34% over the control, because each movement is
parsed from text before summing. This is the item the roadmap defers
("bare NUMERIC columns and numeric aggregates"); the study shows it is a
correctness gap, not only a performance one, and that the int-exponent
design sidesteps it entirely.

**Scale.** The full scan grows linearly with rows on every backend
(pglike 45ms → 540ms → 5.8s and PostgreSQL 10ms → 65ms → 0.8s for 100k →
1M → 10M rows); PostgreSQL is 5–8× faster at the scan, which is the
expected gap between a native engine and SQLite under wazero. Indexed
one-account reads stay in the 65–530µs band on both, rising with table
size as the index deepens. Bulk load through the identical `database/sql`
multi-row INSERT path takes 19–23s for 10M rows on both, so the loader,
not the engine, bounds load time here.

## Verdict

| Question | Answer |
|---|---|
| Is int-exponent exact on pglike? | Yes, at 1k, 10k and 100k accounts, file and `:memory:` |
| Is int-exponent exact on PostgreSQL? | Yes, at every scale |
| Is native-numeric exact on pglike? | No: SUM over a bare NUMERIC column is REAL arithmetic; ~43% of balances drift |
| Cost of the view conversion vs the integer control, pglike | 0–15% on the full scan; noise on an indexed single-account read |
| Cost of the view conversion vs the integer control, PostgreSQL | None measurable (0.90–1.01×); native NUMERIC storage would cost 19–41% |
| Practical for go-luca position views? | Yes. Store BIGINT at the commodity exponent and convert in the view. It is the only design that is exact on both drivers, it is free on PostgreSQL and costs a few percent on pglike, and it is faster than storing NUMERIC natively on both |
