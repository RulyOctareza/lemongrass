// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// postgresCatalogQuery finds the first user object a statement's bare names could resolve to. A bare call to an allowlisted name can still pick a user overload, since search_path resolution prefers an exact argument match over pg_catalog's place on the path; attribute notation `e.leak` calls any one-argument leak(e); and a user operator can win the same way a user overload does. Extension-owned functions and operators are skipped, since only the database owner installs an extension and none of the extension names that reach data (dblink, lo_*, adminpack) are allowlisted; attribute notation keeps them, since any extension function taking a row could then be called.
const postgresCatalogQuery = `SELECT kind, name FROM (
SELECT 'function' AS kind, p.proname::text AS name FROM pg_catalog.pg_proc p
 WHERE p.proname IN (SELECT pg_catalog.jsonb_array_elements_text($1::pg_catalog.jsonb))
   AND p.pronamespace <> 'pg_catalog'::pg_catalog.regnamespace
   AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_depend d
     WHERE d.classid = 'pg_catalog.pg_proc'::pg_catalog.regclass AND d.objid = p.oid AND d.deptype = 'e')
UNION ALL
SELECT 'attribute', p.proname::text FROM pg_catalog.pg_proc p
 WHERE p.proname IN (SELECT pg_catalog.jsonb_array_elements_text($2::pg_catalog.jsonb))
   AND p.pronargs = 1
   AND p.pronamespace <> 'pg_catalog'::pg_catalog.regnamespace
UNION ALL
SELECT 'operator', o.oprname::text FROM pg_catalog.pg_operator o
 WHERE o.oprname IN (SELECT pg_catalog.jsonb_array_elements_text($3::pg_catalog.jsonb))
   AND o.oprnamespace <> 'pg_catalog'::pg_catalog.regnamespace
   AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_depend d
     WHERE d.classid = 'pg_catalog.pg_operator'::pg_catalog.regclass AND d.objid = o.oid AND d.deptype = 'e')
) found LIMIT 1`

// checkPostgresCatalog refuses stmt when a name it uses could resolve to a user object, running inside tx after guardTx so it sees the catalog the statement will resolve against. It fails closed: a lookup error refuses the statement too.
func checkPostgresCatalog(tx *sql.Tx, stmt Statement) error {
	return lookupPostgresCatalog(context.Background(), tx, stmt.Catalog)
}

// catalogQueryer is the part of *sql.Tx the lookup needs.
type catalogQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func lookupPostgresCatalog(ctx context.Context, q catalogQueryer, names CatalogNames) error {
	if len(names.Functions) == 0 && len(names.Attributes) == 0 && len(names.Operators) == 0 {
		return nil
	}
	// Each list goes as one JSON text parameter, so every driver binds plain strings and no name is ever spliced into SQL.
	args := make([]any, 0, 3)
	for _, list := range [][]string{names.Functions, names.Attributes, names.Operators} {
		if list == nil {
			list = []string{}
		}
		js, err := json.Marshal(list)
		if err != nil {
			return errPostgresCatalogCheck(err)
		}
		args = append(args, string(js))
	}
	rows, err := q.QueryContext(ctx, postgresCatalogQuery, args...)
	if err != nil {
		return errPostgresCatalogCheck(err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return errPostgresCatalogCheck(err)
		}
		return nil
	}
	var kind, name string
	if err := rows.Scan(&kind, &name); err != nil {
		return errPostgresCatalogCheck(err)
	}
	if kind == "operator" {
		return fmt.Errorf("dbgate: operator %s is not allowed", name)
	}
	return &ErrFunctionNotAllowed{Name: name}
}

func errPostgresCatalogCheck(err error) error {
	return fmt.Errorf("dbgate: function check failed, the statement was not run: %s", describeDBError(err))
}
