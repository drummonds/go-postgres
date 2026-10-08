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
_Not yet run._
<!-- results:end -->

## Analysis

_Pending the first full run._

## Recommendation for PostgreSQL

_Pending the first full run._
