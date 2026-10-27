// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"reflect"
	"testing"

	"github.com/faizalv/lemongrass/internal/vault"
)

func TestAllowStatementPermitsGrantedSelect(t *testing.T) {
	s := vault.Scope{Tables: []string{"employees"}, Operations: []string{"select"}}
	if err := AllowStatement(s, Statement{Kind: KindSelect, Tables: []string{"employees"}}); err != nil {
		t.Errorf("AllowStatement on a granted select: %v", err)
	}
}

func TestAllowStatementDeniesUngrantedTable(t *testing.T) {
	s := vault.Scope{Tables: []string{"employees"}, Operations: []string{"select"}}
	if err := AllowStatement(s, Statement{Kind: KindSelect, Tables: []string{"salaries"}}); err == nil {
		t.Error("AllowStatement on an ungranted table returned nil, want ErrScopeViolation")
	}
}

func TestAllowStatementDeniesUngrantedKind(t *testing.T) {
	s := vault.Scope{Tables: []string{"employees"}, Operations: []string{"select"}}
	if err := AllowStatement(s, Statement{Kind: KindExplain, Tables: []string{"employees"}}); err == nil {
		t.Error("AllowStatement on an ungranted kind returned nil, want ErrScopeViolation")
	}
}

func TestAllowStatementDeniesOneUngrantedTableAmongSeveral(t *testing.T) {
	s := vault.Scope{Tables: []string{"employees"}, Operations: []string{"select"}}
	if err := AllowStatement(s, Statement{Kind: KindSelect, Tables: []string{"employees", "salaries"}}); err == nil {
		t.Error("AllowStatement with one ungranted table among several returned nil, want ErrScopeViolation")
	}
}

func TestAllowStatementDeniesEverythingWhenEmpty(t *testing.T) {
	var s vault.Scope
	if err := AllowStatement(s, Statement{Kind: KindSelect, Tables: []string{"employees"}}); err == nil {
		t.Error("AllowStatement on a zero-value vault.Scope returned nil, want it to deny by default")
	}
}

func TestAllowStatementIntrospectNeedsShowNotTables(t *testing.T) {
	s := vault.Scope{Tables: nil, Operations: []string{"show"}}
	if err := AllowStatement(s, Statement{Kind: KindIntrospect}); err != nil {
		t.Errorf("AllowStatement on introspect with show granted: %v", err)
	}
}

func TestAllowStatementIntrospectDeniedWithoutShow(t *testing.T) {
	s := vault.Scope{Tables: []string{"employees"}, Operations: []string{"select"}}
	if err := AllowStatement(s, Statement{Kind: KindIntrospect}); err == nil {
		t.Error("AllowStatement on introspect without show granted returned nil, want ErrScopeViolation")
	}
}

func TestAllowStatementMySQLExplainNeedsExplainNotShow(t *testing.T) {
	explainOnly := vault.Scope{Tables: []string{"employees"}, Operations: []string{"explain"}}
	showOnly := vault.Scope{Tables: []string{"employees"}, Operations: []string{"show"}}
	cases := []struct {
		name    string
		scope   vault.Scope
		sql     string
		allowed bool
	}{
		{"explain of a granted table", explainOnly, "EXPLAIN SELECT * FROM employees", true},
		{"explain of an ungranted table", explainOnly, "EXPLAIN SELECT * FROM employees JOIN salaries ON employees.id = salaries.emp_id", false},
		{"show tables without show", explainOnly, "SHOW TABLES", false},
		{"explain without explain", showOnly, "EXPLAIN SELECT * FROM employees", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stmt, err := ClassifyQuery(EngineMySQL, c.sql)
			if err != nil {
				t.Fatalf("ClassifyQuery: %v", err)
			}
			err = AllowStatement(c.scope, stmt)
			if c.allowed && err != nil {
				t.Errorf("AllowStatement: %v", err)
			}
			if !c.allowed && err == nil {
				t.Error("AllowStatement returned nil, want ErrScopeViolation")
			}
		})
	}
}

func TestAllowStatementPermitsGrantedWrite(t *testing.T) {
	s := vault.Scope{Tables: []string{"employees"}, Operations: []string{"insert"}}
	stmt := Statement{Kind: KindInsert, Tables: []string{"employees"}, WriteTables: []string{"employees"}}
	if err := AllowStatement(s, stmt); err != nil {
		t.Errorf("AllowStatement on a granted insert: %v", err)
	}
}

func TestAllowStatementDeniesUngrantedWriteTable(t *testing.T) {
	s := vault.Scope{Tables: []string{"employees"}, Operations: []string{"insert"}}
	stmt := Statement{Kind: KindInsert, Tables: []string{"salaries"}, WriteTables: []string{"salaries"}}
	if err := AllowStatement(s, stmt); err == nil {
		t.Error("AllowStatement on an ungranted write table returned nil, want ErrScopeViolation")
	}
}

func TestAllowStatementDeniesUngrantedWriteKind(t *testing.T) {
	s := vault.Scope{Tables: []string{"employees"}, Operations: []string{"select"}}
	stmt := Statement{Kind: KindInsert, Tables: []string{"employees"}, WriteTables: []string{"employees"}}
	if err := AllowStatement(s, stmt); err == nil {
		t.Error("AllowStatement on an ungranted write kind returned nil, want ErrScopeViolation")
	}
}

func TestAllowStatementWriteRequiresSelectForReadTable(t *testing.T) {
	s := vault.Scope{Tables: []string{"a", "b"}, Operations: []string{"insert"}}
	stmt := Statement{Kind: KindInsert, Tables: []string{"a", "b"}, WriteTables: []string{"a"}}
	if err := AllowStatement(s, stmt); err == nil {
		t.Error("AllowStatement writing a with an incidental read of b, no select granted, returned nil, want ErrScopeViolation")
	}
}

func TestAllowStatementWritePermitsReadTableWithSelectGranted(t *testing.T) {
	s := vault.Scope{Tables: []string{"a", "b"}, Operations: []string{"insert", "select"}}
	stmt := Statement{Kind: KindInsert, Tables: []string{"a", "b"}, WriteTables: []string{"a"}}
	if err := AllowStatement(s, stmt); err != nil {
		t.Errorf("AllowStatement writing a with an incidental read of b, select granted: %v", err)
	}
}

func TestAllowStatementWriteDeniesReadTableUngranted(t *testing.T) {
	s := vault.Scope{Tables: []string{"a"}, Operations: []string{"insert", "select"}}
	stmt := Statement{Kind: KindInsert, Tables: []string{"a", "b"}, WriteTables: []string{"a"}}
	if err := AllowStatement(s, stmt); err == nil {
		t.Error("AllowStatement with select granted but b itself ungranted returned nil, want ErrScopeViolation")
	}
}

func TestFilterTableListDropsUngrantedTables(t *testing.T) {
	s := vault.Scope{Tables: []string{"menu", "sub_menu"}}
	result := QueryResult{
		Columns: []string{"Tables_in_db"},
		Rows: [][]interface{}{
			{"menu"}, {"sub_menu"}, {"users"}, {"mst_pemanen"},
		},
	}
	got := FilterTableList(s, result)
	want := [][]interface{}{{"menu"}, {"sub_menu"}}
	if !reflect.DeepEqual(got.Rows, want) {
		t.Errorf("FilterTableList rows = %v, want %v", got.Rows, want)
	}
}

func TestFilterTableListEmptyScopeDropsEverything(t *testing.T) {
	var s vault.Scope
	result := QueryResult{Columns: []string{"Tables_in_db"}, Rows: [][]interface{}{{"menu"}}}
	got := FilterTableList(s, result)
	if len(got.Rows) != 0 {
		t.Errorf("FilterTableList with empty scope returned %v, want no rows", got.Rows)
	}
}
