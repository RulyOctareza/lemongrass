// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// mysqlCatalogName is the only shape a name may have to be inlined into the lookup, which every allowlisted name has.
var mysqlCatalogName = regexp.MustCompile(`^[a-z0-9_]+$`)

// mysqlCatalogRoutines finds a stored function sharing a called name. The server's own schemas are left out because MySQL 8 ships sys.format_bytes and sys.ps_thread_id, read-only formatters that would otherwise refuse the performance-view helpers on every server. ROUTINES lists only routines the user holds a privilege on, and calling one needs EXECUTE, so every function the channel could reach is listed.
const mysqlCatalogRoutines = `SELECT LOWER(ROUTINE_NAME) FROM information_schema.ROUTINES WHERE ROUTINE_TYPE = 'FUNCTION' AND ROUTINE_SCHEMA NOT IN ('sys', 'mysql', 'performance_schema', 'information_schema') AND LOWER(ROUTINE_NAME) IN (%s)`

// mysqlCatalogLoadable finds a loadable function sharing a called name. MariaDB has no such table, so there only the routines half runs.
const mysqlCatalogLoadable = ` UNION ALL SELECT LOWER(UDF_NAME) FROM performance_schema.user_defined_functions WHERE LOWER(UDF_NAME) IN (%s)`

// checkMySQLCatalog refuses a statement when any function it calls by an allowlisted name could resolve to a stored or loadable function instead of the built-in, as happens when the server lacks that built-in. It runs inside tx before the caller's SQL and fails closed: a lookup that errors refuses the statement.
func checkMySQLCatalog(tx *sql.Tx, stmt Statement) error {
	names := stmt.Catalog.Functions
	if len(names) == 0 {
		return nil
	}
	quoted := make([]string, len(names))
	for i, name := range names {
		if !mysqlCatalogName.MatchString(name) {
			return &ErrFunctionNotAllowed{Name: name}
		}
		quoted[i] = "'" + name + "'"
	}
	list := strings.Join(quoted, ", ")
	routines := fmt.Sprintf(mysqlCatalogRoutines, list)

	found, err := mysqlCatalogLookup(tx, routines+fmt.Sprintf(mysqlCatalogLoadable, list))
	if err != nil {
		mariaDB, verr := mysqlIsMariaDB(tx)
		if verr != nil || !mariaDB {
			return mysqlCatalogFailed(err)
		}
		if found, err = mysqlCatalogLookup(tx, routines); err != nil {
			return mysqlCatalogFailed(err)
		}
	}
	if found != "" {
		return &ErrFunctionNotAllowed{Name: found}
	}
	return nil
}

func mysqlCatalogFailed(err error) error {
	return fmt.Errorf("dbgate: function check failed, the statement was not run: %s", describeDBError(err))
}

// mysqlCatalogLookup returns the first name query finds, or "" when it finds none.
func mysqlCatalogLookup(tx *sql.Tx, query string) (string, error) {
	var name string
	err := tx.QueryRow(query + " LIMIT 1").Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return name, err
}

// mysqlIsMariaDB is asked only after the full lookup failed, so a MySQL server pays no extra round trip.
func mysqlIsMariaDB(tx *sql.Tx) (bool, error) {
	var version string
	if err := tx.QueryRow("SELECT VERSION()").Scan(&version); err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(version), "mariadb"), nil
}
