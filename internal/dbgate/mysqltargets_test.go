// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"reflect"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/vault"
)

func TestMySQLDeleteTargetsResolveThroughAliases(t *testing.T) {
	cases := []struct {
		sql    string
		writes []string
	}{
		{"DELETE s FROM employees e JOIN secrets s ON e.id = s.id WHERE e.id = 1", []string{"secrets"}},
		{"DELETE S FROM employees e JOIN secrets s ON e.id = s.id WHERE e.id = 1", []string{"secrets"}},
		{"DELETE e, s FROM employees e JOIN secrets s ON e.id = s.id WHERE e.id = 1", []string{"employees", "secrets"}},
		{"DELETE secrets FROM employees JOIN secrets ON employees.id = secrets.id WHERE employees.id = 1", []string{"secrets"}},
		{"DELETE secrets FROM employees secrets JOIN salaries s ON secrets.id = s.id WHERE s.id = 1", []string{"employees"}},
		// Swapped aliases: MySQL resolves a target by alias first, so "secrets" here is the employees table.
		{"DELETE secrets FROM employees secrets JOIN secrets employees ON secrets.id = employees.id WHERE secrets.id = 1", []string{"employees"}},
		{"DELETE FROM employees WHERE id = 1", []string{"employees"}},
	}
	for _, c := range cases {
		stmt := classifyFor(t, EngineMySQL, c.sql)
		if !reflect.DeepEqual(stmt.WriteTables, c.writes) {
			t.Errorf("ClassifyQuery(%q).WriteTables = %v, want %v", c.sql, stmt.WriteTables, c.writes)
		}
	}
}

func TestMySQLDeleteTargetsRefusedWhenUnresolved(t *testing.T) {
	cases := []struct {
		sql  string
		want string
	}{
		{"DELETE x FROM employees e JOIN secrets s ON e.id = s.id WHERE e.id = 1", "does not name a table"},
		{"DELETE secrets FROM employees e JOIN secrets s ON e.id = s.id WHERE e.id = 1", "does not name a table"},
		{"DELETE s FROM employees e WHERE e.id IN (SELECT id FROM secrets s)", "does not name a table"},
		{"DELETE s FROM employees s JOIN secrets S ON s.id = S.id WHERE s.id = 1", "ambiguous"},
		{"UPDATE employees e, secrets E SET e.name = 'x' WHERE e.id = E.id", "ambiguous"},
	}
	for _, c := range cases {
		_, err := ClassifyQuery(EngineMySQL, c.sql)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("ClassifyQuery(%q) = %v, want an error containing %q", c.sql, err, c.want)
		}
	}
}

func TestMySQLDeleteThroughAliasCheckedAsDelete(t *testing.T) {
	stmt := classifyFor(t, EngineMySQL, "DELETE s FROM employees e JOIN secrets s ON e.id = s.id WHERE e.id = 1")
	scope := vault.Scope{Tables: []string{"employees", "secrets"}, Operations: []string{"select", "delete"}}
	if err := AllowStatement(scope, stmt); err != nil {
		t.Fatalf("AllowStatement: %v", err)
	}
	if err := AllowStatement(vault.Scope{Tables: []string{"employees"}, Operations: []string{"select", "delete"}}, stmt); err == nil {
		t.Error("a delete through an alias of an ungranted table passed")
	}
}
