# UUID Keys: declared UUID vs TEXT on PostgreSQL

What does it cost on PostgreSQL to declare a UUID key column as `TEXT`
(or `VARCHAR(36)`) rather than `UUID`? go-luca and gobank do this today
because pglike stores both as TEXT, so the choice looked free in
development. This study measures the difference where it is not free.

**Status**: results below are from the run recorded in the results header.
Program: `research/cmd/uuid-keys/` (in the `research` Go module). Run with
`task research:uuid-keys`; set `BENCH_PG_DSN` for the native pass.

## Why

pglike maps `UUID` to TEXT inside SQLite and keeps the `/*pg:uuid*/`
annotation, so a schema may say `UUID` and behave identically in
development. On PostgreSQL the two types differ in storage (16 bytes
against 37), in how the B-tree compares keys (memcmp against a
collation-aware string compare), and in validation. Every ledger row
carries at least two keys (its own id and an account reference), and the
foreign-key index is the one balance queries walk, so the cost multiplies.

## Vocabulary

| Term | Meaning |
|---|---|
| Key | A column holding a v4 UUID: the accounts primary key, the movements primary key, the movements foreign key |
| Design | The declared type of every key column |
| Movement | One ledger entry with its own key, an account key and an amount |
| Backend | `pglike-file` (control: both designs are TEXT there), `postgres` |

## Designs

| Design | Key type | Where |
|---|---|---|
| uuid | `UUID` | both drivers |
| text | `TEXT` | both drivers; what the apps declare today |
| text-c | `TEXT COLLATE "C"` | PostgreSQL only; separates the storage cost from the collation cost |

Same tables for every design, same seeded UUID values for every design:

```sql
CREATE TABLE accounts_<design>  (id <key> PRIMARY KEY, ref VARCHAR(20) NOT NULL);
CREATE TABLE movements_<design> (id <key> PRIMARY KEY,
                                 account_id <key> NOT NULL REFERENCES accounts_<design>(id),
                                 amount BIGINT NOT NULL);
CREATE INDEX movements_<design>_account ON movements_<design> (account_id);
```

## Method

- n accounts × 100 movements each, canonical lower-case v4 UUIDs from a
  fixed seed, interleaved by account as a ledger would receive them.
- Load: multi-row `INSERT` (500 rows per statement) through prepared
  statements in one transaction, accounts then movements, the same path on
  both drivers; `ANALYZE` on PostgreSQL after load.
- Sizes: `pg_table_size` and `pg_indexes_size` of the movements table on
  PostgreSQL; the whole database file on pglike-file, each design loaded
  into a fresh file.
- By account: `SELECT count(*), sum(amount) FROM movements WHERE account_id = $1`,
  random accounts, the foreign-key index walk a balance needs.
- By id: `SELECT amount FROM movements WHERE id = $1`, random movements,
  the primary-key point lookup.
- Join: `SELECT count(*) FROM movements m JOIN accounts a ON a.id = m.account_id WHERE a.ref <> ''`,
  every movement matched to its account on the key.
- No warm-up; p50 and p99 over the iterations stated in the results header.

## Results

<!-- results:start -->
**Run:** 2026-10-13 23:32 · linux/amd64 · go1.26.4 · 100 movements per account · point queries ×200 · join ×3

- pglike-file: pglike (working tree) over SQLite 3.53.4
- postgres: PostgreSQL 16.15

### 1_000 accounts (100_000 movements)

| Backend | Design | Load | Table | Indexes | By account p50 | By account p99 | By id p50 | By id p99 | Join p50 |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| pglike-file | uuid | 1.27s | 18.0 MB | - | 253.3us | 437.4us | 25.7us | 142.2us | 49.57ms |
| pglike-file | text | 1.27s | 18.0 MB | - | 245.3us | 445.8us | 24.3us | 36.5us | 50.95ms |
| postgres | uuid | 1.84s | 6.5 MB | 5.0 MB | 127.3us | 253.1us | 55.0us | 92.4us | 10.57ms |
| postgres | text | 1.94s | 11.2 MB | 8.3 MB | 121.4us | 313.5us | 57.1us | 128.1us | 12.98ms |
| postgres | text-c | 1.81s | 11.2 MB | 8.3 MB | 110.2us | 357.7us | 50.1us | 81.4us | 12.06ms |

### 10_000 accounts (1_000_000 movements)

| Backend | Design | Load | Table | Indexes | By account p50 | By account p99 | By id p50 | By id p99 | Join p50 |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| pglike-file | uuid | 21.82s | 180.7 MB | - | 279.0us | 731.7us | 31.6us | 49.0us | 638.54ms |
| pglike-file | text | 21.97s | 180.7 MB | - | 275.5us | 513.1us | 26.3us | 43.9us | 639.29ms |
| postgres | uuid | 18.86s | 65.1 MB | 47.0 MB | 149.6us | 268.4us | 70.9us | 198.0us | 40.94ms |
| postgres | text | 20.88s | 111.7 MB | 81.6 MB | 403.4us | 802.4us | 63.4us | 364.2us | 58.81ms |
| postgres | text-c | 19.74s | 111.7 MB | 81.6 MB | 421.3us | 736.3us | 70.0us | 280.8us | 56.15ms |

### 100_000 accounts (10_000_000 movements)

| Backend | Design | Load | Table | Indexes | By account p50 | By account p99 | By id p50 | By id p99 | Join p50 |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| pglike-file | uuid | 259.89s | 1.77 GB | - | 3.00ms | 5.74ms | 56.5us | 194.1us | 7.67s |
| pglike-file | text | 263.30s | 1.77 GB | - | 2.40ms | 3.37ms | 42.3us | 121.6us | 6.82s |
| postgres | uuid | 227.37s | 651.2 MB | 478.6 MB | 3.63ms | 4.64ms | 137.2us | 276.3us | 459.29ms |
| postgres | text | 276.35s | 1.09 GB | 818.9 MB | 4.65ms | 5.49ms | 166.5us | 239.6us | 1.28s |
| postgres | text-c | 265.78s | 1.09 GB | 818.9 MB | 4.54ms | 5.64ms | 159.4us | 213.9us | 997.22ms |

On pglike-file, Table is the whole database file (both tables and their indexes) and Indexes is not separated.

### TEXT relative to UUID (p50)

| Backend | Accounts | Design | Table | Indexes | By account | By id | Join |
|---|---:|---|---:|---:|---:|---:|---:|
| pglike-file | 1_000 | text | 1.00x | n/a | 0.97x | 0.95x | 1.03x |
| pglike-file | 10_000 | text | 1.00x | n/a | 0.99x | 0.83x | 1.00x |
| pglike-file | 100_000 | text | 1.00x | n/a | 0.80x | 0.75x | 0.89x |
| postgres | 1_000 | text | 1.71x | 1.67x | 0.95x | 1.04x | 1.23x |
| postgres | 1_000 | text-c | 1.71x | 1.67x | 0.87x | 0.91x | 1.14x |
| postgres | 10_000 | text | 1.71x | 1.74x | 2.70x | 0.89x | 1.44x |
| postgres | 10_000 | text-c | 1.71x | 1.74x | 2.82x | 0.99x | 1.37x |
| postgres | 100_000 | text | 1.71x | 1.71x | 1.28x | 1.21x | 2.78x |
| postgres | 100_000 | text-c | 1.71x | 1.71x | 1.25x | 1.16x | 2.17x |
<!-- results:end -->

## Analysis

_Pending the first full run._

## Recommendation for PostgreSQL

_Pending the first full run._
