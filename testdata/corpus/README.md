# Conformance corpus

Language-neutral behavior tests shared between go-postgres and its Python
twin, [py-postgres](https://git.bytestone.uk/hum3/py-postgres). Each case
feeds PostgreSQL-dialect SQL to an implementation and checks the *executed
results* — never the translated SQLite text — so each implementation is
free to translate differently as long as it behaves the same.

Every case runs against a fresh in-memory database.

## File format

Fixture files are `*.sql`. Directive lines start with `-- `; any other
`--` line is a comment. Lines that don't start with `--` belong to the
most recent section directive.

```
-- case: <unique name within the file>     start a new case
-- skip: <reason>                          parse but don't run; documents known gaps
-- setup:                                  PG-dialect SQL statements, executed as one batch
-- query:                                  exactly one statement; its result is checked
-- params: v1|v2|...                       bind values for $1..$n in the query
-- expect:                                 expected rows, one per line, cells separated by |
-- expect-match:                           like expect, but each cell is an anchored regex
-- expect-error: <SQLSTATE>                the query must fail with this error code
```

A case needs `query:` and exactly one of `expect:`, `expect-match:`, or
`expect-error:`. An `expect:` section with no rows means the query must
return zero rows. Rows are compared in order — use `ORDER BY` whenever a
query returns more than one row.

## Cell rendering

Result values are rendered to text before comparison:

| value     | rendered as                       |
|-----------|-----------------------------------|
| NULL      | `NULL`                            |
| integer   | decimal digits                    |
| float     | shortest round-trip decimal form  |
| text/blob | the raw text                      |
| timestamp | `YYYY-MM-DD HH:MM:SS` (UTC)       |

`params:` cells are parsed the same way: `NULL`, else integer, else
float, else text.

Limitations (keep fixtures within these): cell values must not contain
`|` or newlines, and text cells must not literally be `NULL`.
