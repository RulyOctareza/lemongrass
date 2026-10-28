// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"errors"
	"reflect"
	"testing"

	"github.com/faizalv/lemongrass/internal/vault"
)

func TestMySQLFromlessSelectTouchesNoTable(t *testing.T) {
	selectOnly := vault.Scope{Tables: []string{"employees"}, Operations: []string{"select", "explain", "insert"}}
	for _, sql := range []string{
		"SELECT NOW()",
		"SELECT 1",
		"SELECT JSON_EXTRACT('{\"a\":1}', '$.a')",
		"SELECT 1 FROM dual",
		"SELECT 1 FROM DUAL",
		"SELECT 1 FROM Dual WHERE 1 = 1",
		"SELECT (SELECT 1) AS x",
		"SELECT \"dual\"",
		"EXPLAIN SELECT 1",
		"INSERT INTO employees (id) SELECT 1",
		"INSERT INTO employees (id) SELECT 1 FROM dual",
	} {
		stmt := classifyFor(t, EngineMySQL, sql)
		for _, tbl := range stmt.Tables {
			if tbl == "dual" {
				t.Errorf("ClassifyQuery(%q).Tables = %v, want no dual", sql, stmt.Tables)
			}
		}
		if err := AllowStatement(selectOnly, stmt); err != nil {
			t.Errorf("AllowStatement(%q): %v", sql, err)
		}
	}
}

func TestMySQLBacktickedDualIsARealTable(t *testing.T) {
	for _, sql := range []string{
		"SELECT * FROM `dual`",
		"SELECT * FROM `DUAL` d",
		"SELECT id FROM employees WHERE id IN (SELECT id FROM `dual`)",
		"SELECT e.id FROM employees e JOIN `dual` d ON d.id = e.id",
		"SELECT 1 FROM dual WHERE 1 IN (SELECT id FROM `dual`)",
		"SELECT '`dual`' FROM dual",
	} {
		stmt := classifyFor(t, EngineMySQL, sql)
		if !contains(stmt.Tables, "dual") && !contains(stmt.Tables, "DUAL") {
			t.Errorf("ClassifyQuery(%q).Tables = %v, want the real dual", sql, stmt.Tables)
			continue
		}
		var violation *ErrScopeViolation
		if err := AllowStatement(vault.Scope{Tables: []string{"employees"}, Operations: []string{"select"}}, stmt); !errors.As(err, &violation) {
			t.Errorf("AllowStatement(%q) without a dual grant = %v, want a scope violation", sql, err)
		}
	}
	stmt := classifyFor(t, EngineMySQL, "SELECT * FROM `dual`")
	if err := AllowStatement(vault.Scope{Tables: []string{"dual"}, Operations: []string{"select"}}, stmt); err != nil {
		t.Errorf("AllowStatement with a dual grant: %v", err)
	}
	stmt = classifyFor(t, EngineMySQL, "SELECT * FROM appdb.dual")
	if !reflect.DeepEqual(stmt.Tables, []string{"appdb.dual"}) {
		t.Errorf("qualified dual: Tables = %v, want [appdb.dual]", stmt.Tables)
	}
}

func TestMySQLFromlessSelectFunctionsStillChecked(t *testing.T) {
	for _, c := range []struct{ sql, name string }{
		{"SELECT SLEEP(1)", "sleep"},
		{"SELECT BENCHMARK(1000000, MD5('x'))", "benchmark"},
		{"SELECT appdb.leak()", "appdb.leak"},
		{"SELECT 1 FROM dual WHERE GET_LOCK('x', 1)", "get_lock"},
	} {
		_, err := ClassifyQuery(EngineMySQL, c.sql)
		var notAllowed *ErrFunctionNotAllowed
		if !errors.As(err, &notAllowed) || notAllowed.Name != c.name {
			t.Errorf("ClassifyQuery(%q) = %v, want function %s refused", c.sql, err, c.name)
		}
	}
	stmt := classifyFor(t, EngineMySQL, "SELECT UPPER('a'), NOW()")
	if !reflect.DeepEqual(stmt.Catalog.Functions, []string{"now", "upper"}) {
		t.Errorf("Catalog.Functions = %v, want [now upper] for the catalog check", stmt.Catalog.Functions)
	}
}
