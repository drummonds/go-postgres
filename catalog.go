package pglike

// catalogViews are the TEMP VIEW DDLs installed on every new connection so
// that PG-style catalog queries (information_schema.*, pg_indexes) work
// against the underlying SQLite database.
//
// Names are mangled to a single SQLite identifier (`_pglike_<schema>_<view>`)
// because SQLite resolves `<a>.<b>` as `<attached-db>.<table>`, and our
// catalog views need to read from `main.sqlite_master`. The translator
// rewrites canonical PG references (e.g. `information_schema.columns`) to
// these mangled names so callers write portable SQL.
//
// Coverage is the minimum needed by lofidb's table/column/FK/index browsing.
// PG's information_schema is much larger; we fill it in as needs arise.
var catalogViews = []string{
	// Tables: BASE TABLE for user tables, VIEW for views, internal helpers hidden.
	`CREATE TEMP VIEW _pglike_information_schema_tables AS
	SELECT
		'main' AS table_catalog,
		'public' AS table_schema,
		m.name AS table_name,
		CASE m.type WHEN 'view' THEN 'VIEW' ELSE 'BASE TABLE' END AS table_type
	FROM sqlite_master m
	WHERE m.type IN ('table','view')
	  AND m.name NOT LIKE 'sqlite_%'
	  AND m.name NOT LIKE '\_pglike\_%' ESCAPE '\'
	  AND m.name <> '_sequences'`,

	// Columns: pragma_table_info as a TVF correlated against sqlite_master.
	// is_nullable matches PG's 'YES'/'NO'. ordinal_position is 1-based.
	`CREATE TEMP VIEW _pglike_information_schema_columns AS
	SELECT
		'main' AS table_catalog,
		'public' AS table_schema,
		m.name AS table_name,
		p.name AS column_name,
		p.cid + 1 AS ordinal_position,
		p.dflt_value AS column_default,
		CASE WHEN p.pk > 0 OR p."notnull" <> 0 THEN 'NO' ELSE 'YES' END AS is_nullable,
		p.type AS data_type,
		p.type AS udt_name
	FROM sqlite_master m, pragma_table_info(m.name) p
	WHERE m.type = 'table'
	  AND m.name NOT LIKE 'sqlite_%'
	  AND m.name NOT LIKE '\_pglike\_%' ESCAPE '\'
	  AND m.name <> '_sequences'`,

	// table_constraints: one row per PK/FK/UNIQUE constraint per table.
	// Constraint names are synthesised — SQLite doesn't preserve them.
	//   PK   → <table>_pkey
	//   FK   → <table>_fk_<id>      (id from pragma_foreign_key_list)
	//   UNIQ → <pragma_index_list.name>
	`CREATE TEMP VIEW _pglike_information_schema_table_constraints AS
	SELECT 'main' AS constraint_catalog, 'public' AS constraint_schema,
	       m.name || '_pkey' AS constraint_name,
	       'main' AS table_catalog, 'public' AS table_schema, m.name AS table_name,
	       'PRIMARY KEY' AS constraint_type
	FROM sqlite_master m
	WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%'
	  AND m.name NOT LIKE '\_pglike\_%' ESCAPE '\'
	  AND m.name <> '_sequences'
	  AND EXISTS (SELECT 1 FROM pragma_table_info(m.name) p WHERE p.pk > 0)
	UNION ALL
	SELECT 'main', 'public',
	       m.name || '_fk_' || fk.id,
	       'main', 'public', m.name,
	       'FOREIGN KEY'
	FROM sqlite_master m, pragma_foreign_key_list(m.name) fk
	WHERE m.type = 'table' AND fk.seq = 0
	  AND m.name NOT LIKE 'sqlite_%'
	  AND m.name NOT LIKE '\_pglike\_%' ESCAPE '\'
	UNION ALL
	SELECT 'main', 'public',
	       il.name,
	       'main', 'public', m.name,
	       'UNIQUE'
	FROM sqlite_master m, pragma_index_list(m.name) il
	WHERE m.type = 'table' AND il."unique" = 1 AND il.origin = 'u'
	  AND m.name NOT LIKE 'sqlite_%'
	  AND m.name NOT LIKE '\_pglike\_%' ESCAPE '\'`,

	// key_column_usage: one row per column in a PK or FK.
	// ordinal_position is 1-based within the constraint.
	`CREATE TEMP VIEW _pglike_information_schema_key_column_usage AS
	SELECT 'main' AS constraint_catalog, 'public' AS constraint_schema,
	       m.name || '_pkey' AS constraint_name,
	       'main' AS table_catalog, 'public' AS table_schema, m.name AS table_name,
	       p.name AS column_name,
	       p.pk AS ordinal_position,
	       NULL AS position_in_unique_constraint
	FROM sqlite_master m, pragma_table_info(m.name) p
	WHERE m.type = 'table' AND p.pk > 0
	  AND m.name NOT LIKE 'sqlite_%'
	  AND m.name NOT LIKE '\_pglike\_%' ESCAPE '\'
	  AND m.name <> '_sequences'
	UNION ALL
	SELECT 'main', 'public',
	       m.name || '_fk_' || fk.id,
	       'main', 'public', m.name,
	       fk."from",
	       fk.seq + 1,
	       fk.seq + 1
	FROM sqlite_master m, pragma_foreign_key_list(m.name) fk
	WHERE m.type = 'table'
	  AND m.name NOT LIKE 'sqlite_%'
	  AND m.name NOT LIKE '\_pglike\_%' ESCAPE '\'`,

	// referential_constraints: FK metadata (update/delete rules).
	// unique_constraint_name points at the parent table's PK by convention.
	`CREATE TEMP VIEW _pglike_information_schema_referential_constraints AS
	SELECT 'main' AS constraint_catalog, 'public' AS constraint_schema,
	       m.name || '_fk_' || fk.id AS constraint_name,
	       'main' AS unique_constraint_catalog,
	       'public' AS unique_constraint_schema,
	       fk."table" || '_pkey' AS unique_constraint_name,
	       'NONE' AS match_option,
	       fk.on_update AS update_rule,
	       fk.on_delete AS delete_rule
	FROM sqlite_master m, pragma_foreign_key_list(m.name) fk
	WHERE m.type = 'table' AND fk.seq = 0
	  AND m.name NOT LIKE 'sqlite_%'
	  AND m.name NOT LIKE '\_pglike\_%' ESCAPE '\'`,

	// constraint_column_usage: columns referenced by a constraint.
	// For FK rows this points at the parent table's column;
	// for PK rows this points at the PK columns themselves (matches PG).
	`CREATE TEMP VIEW _pglike_information_schema_constraint_column_usage AS
	SELECT 'main' AS table_catalog, 'public' AS table_schema,
	       fk."table" AS table_name,
	       fk."to" AS column_name,
	       'main' AS constraint_catalog, 'public' AS constraint_schema,
	       m.name || '_fk_' || fk.id AS constraint_name
	FROM sqlite_master m, pragma_foreign_key_list(m.name) fk
	WHERE m.type = 'table'
	  AND m.name NOT LIKE 'sqlite_%'
	  AND m.name NOT LIKE '\_pglike\_%' ESCAPE '\'
	UNION ALL
	SELECT 'main', 'public', m.name, p.name,
	       'main', 'public', m.name || '_pkey'
	FROM sqlite_master m, pragma_table_info(m.name) p
	WHERE m.type = 'table' AND p.pk > 0
	  AND m.name NOT LIKE 'sqlite_%'
	  AND m.name NOT LIKE '\_pglike\_%' ESCAPE '\'
	  AND m.name <> '_sequences'`,

	// pg_indexes: PG's view listing indexes with their DDL.
	// SQLite stores the original CREATE INDEX text in sqlite_master.sql.
	`CREATE TEMP VIEW _pglike_pg_indexes AS
	SELECT 'public' AS schemaname,
	       m.tbl_name AS tablename,
	       m.name AS indexname,
	       NULL AS tablespace,
	       m.sql AS indexdef
	FROM sqlite_master m
	WHERE m.type = 'index'
	  AND m.name NOT LIKE 'sqlite_%'
	  AND m.tbl_name NOT LIKE '\_pglike\_%' ESCAPE '\'
	  AND m.tbl_name <> '_sequences'`,

	// pg_index_columns: pglike-specific helper exposing index columns
	// in a flat shape. PG users normally join pg_index/pg_class/pg_attribute;
	// we expose the same data via pragma_index_info as a TVF.
	`CREATE TEMP VIEW _pglike_pg_index_columns AS
	SELECT 'public' AS schemaname,
	       m.tbl_name AS tablename,
	       m.name AS indexname,
	       ic.name AS column_name,
	       ic.seqno + 1 AS ordinal_position,
	       il."unique" AS is_unique
	FROM sqlite_master m
	     JOIN pragma_index_list(m.tbl_name) il ON il.name = m.name,
	     pragma_index_info(m.name) ic
	WHERE m.type = 'index'
	  AND m.name NOT LIKE 'sqlite_%'
	  AND m.tbl_name NOT LIKE '\_pglike\_%' ESCAPE '\'`,
}

// installCatalogViews creates the PG-compatible catalog views on the given
// connection. Views are TEMP, so they live for the lifetime of the
// connection and are dropped automatically on close.
func installCatalogViews(c *conn) error {
	for _, ddl := range catalogViews {
		if err := c.execDirect(ddl); err != nil {
			return err
		}
	}
	return nil
}
