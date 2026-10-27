// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"errors"
	"reflect"
	"testing"

	"github.com/faizalv/lemongrass/internal/vault"
)

const digestTable = "performance_schema.events_statements_summary_by_digest"

func TestClassifyQueryKeepsNonSystemQualifier(t *testing.T) {
	cases := []struct {
		engine Engine
		sql    string
		tables []string
	}{
		{EngineMySQL, "SELECT * FROM otherdb.orders", []string{"otherdb.orders"}},
		{EngineMySQL, "SELECT * FROM `otherdb`.`orders` o JOIN salaries s ON o.id = s.oid", []string{"otherdb.orders", "salaries"}},
		{EnginePostgres, "SELECT * FROM public.orders", []string{"public.orders"}},
		{EnginePostgres, "SELECT * FROM otherschema.orders", []string{"otherschema.orders"}},
		{EnginePostgres, "INSERT INTO otherschema.orders (id) VALUES (1)", []string{"otherschema.orders"}},
	}
	for _, c := range cases {
		got, err := ClassifyQuery(c.engine, c.sql)
		if err != nil {
			t.Fatalf("ClassifyQuery(%q): %v", c.sql, err)
		}
		if !reflect.DeepEqual(got.Tables, c.tables) {
			t.Errorf("ClassifyQuery(%q).Tables = %v, want %v", c.sql, got.Tables, c.tables)
		}
	}
}

func TestClassifyQueryKeepsSystemQualifier(t *testing.T) {
	cases := []struct {
		engine Engine
		sql    string
		tables []string
	}{
		{EngineMySQL, "SELECT * FROM performance_schema.threads", []string{"performance_schema.threads"}},
		{EngineMySQL, "SELECT * FROM PERFORMANCE_SCHEMA.Threads", []string{"performance_schema.threads"}},
		{EngineMySQL, "SELECT * FROM `sys`.`statement_analysis` s JOIN orders o ON o.id = s.id", []string{"orders", "sys.statement_analysis"}},
		{EngineMySQL, "SELECT * FROM mysql.user", []string{"mysql.user"}},
		{EngineMySQL, "SELECT * FROM information_schema.tables", []string{"information_schema.tables"}},
		{EnginePostgres, "SELECT * FROM pg_catalog.pg_locks", []string{"pg_catalog.pg_locks"}},
		{EnginePostgres, "SELECT * FROM pg_stat_statements", []string{"pg_stat_statements"}},
		{EnginePostgres, "SELECT * FROM information_schema.tables", []string{"information_schema.tables"}},
	}
	for _, c := range cases {
		got, err := ClassifyQuery(c.engine, c.sql)
		if err != nil {
			t.Fatalf("ClassifyQuery(%q): %v", c.sql, err)
		}
		if !reflect.DeepEqual(got.Tables, c.tables) {
			t.Errorf("ClassifyQuery(%q).Tables = %v, want %v", c.sql, got.Tables, c.tables)
		}
		if got.Engine != c.engine {
			t.Errorf("ClassifyQuery(%q).Engine = %q, want %q", c.sql, got.Engine, c.engine)
		}
	}
}

func TestClassifyQueryRejectsRestrictedColumn(t *testing.T) {
	for _, sql := range []string{
		"SELECT * FROM performance_schema.events_statements_summary_by_digest",
		"SELECT d.* FROM performance_schema.events_statements_summary_by_digest d",
		"SELECT query_sample_text FROM performance_schema.events_statements_summary_by_digest",
		"SELECT QUERY_SAMPLE_TEXT AS q FROM performance_schema.events_statements_summary_by_digest",
		"SELECT LEFT(d.`Query_Sample_Text`, 10) FROM performance_schema.events_statements_summary_by_digest d",
		"SELECT digest FROM performance_schema.events_statements_summary_by_digest ORDER BY query_sample_text",
		"SELECT x.q FROM (SELECT * FROM performance_schema.events_statements_summary_by_digest) x",
		"SELECT a, b FROM orders UNION ALL SELECT * FROM performance_schema.events_statements_summary_by_digest",
		"SELECT * FROM performance_schema.data_locks",
		"SELECT lock_data FROM performance_schema.data_locks",
	} {
		_, err := ClassifyQuery(EngineMySQL, sql)
		var restricted *ErrRestrictedColumn
		if !errors.As(err, &restricted) {
			t.Errorf("ClassifyQuery(%q) error = %v, want ErrRestrictedColumn", sql, err)
		}
	}
}

func TestClassifyQueryAllowsRestrictedTableWithNamedColumns(t *testing.T) {
	for _, sql := range []string{
		"SELECT digest_text, count_star, sum_timer_wait FROM performance_schema.events_statements_summary_by_digest ORDER BY sum_timer_wait DESC LIMIT 10",
		"SELECT COUNT(*) FROM performance_schema.events_statements_summary_by_digest",
		"SELECT schema_name, COUNT(*) FROM performance_schema.events_statements_summary_by_digest GROUP BY schema_name",
		"SELECT * FROM performance_schema.table_io_waits_summary_by_table",
		"SELECT * FROM sys.statement_analysis",
	} {
		if _, err := ClassifyQuery(EngineMySQL, sql); err != nil {
			t.Errorf("ClassifyQuery(%q): %v", sql, err)
		}
	}
}

func TestClassifyQueryStarWithoutRestrictedTableIsUnaffected(t *testing.T) {
	if _, err := ClassifyQuery(EngineMySQL, "SELECT * FROM orders WHERE created > (SELECT MAX(x) FROM salaries)"); err != nil {
		t.Errorf("ClassifyQuery: %v", err)
	}
}

func TestCheckRestrictedColumnsPostgres(t *testing.T) {
	restrictedViews := perfViews[EnginePostgres]
	restrictedViews["pg_stat_probe"] = []string{"secret"}
	defer delete(restrictedViews, "pg_stat_probe")

	for _, sql := range []string{
		"SELECT * FROM pg_stat_probe",
		"SELECT p.* FROM pg_stat_probe p",
		"SELECT p.secret FROM pg_stat_probe p",
		"SELECT SECRET AS s FROM pg_stat_probe",
	} {
		_, err := ClassifyQuery(EnginePostgres, sql)
		var restricted *ErrRestrictedColumn
		if !errors.As(err, &restricted) {
			t.Errorf("ClassifyQuery(%q) error = %v, want ErrRestrictedColumn", sql, err)
		}
	}
	for _, sql := range []string{
		"SELECT COUNT(*) FROM pg_stat_probe",
		"SELECT other FROM pg_stat_probe",
	} {
		if _, err := ClassifyQuery(EnginePostgres, sql); err != nil {
			t.Errorf("ClassifyQuery(%q): %v", sql, err)
		}
	}
}

func TestAllowStatementPerformanceGrant(t *testing.T) {
	withGrant := vault.Scope{Tables: []string{"orders"}, Operations: []string{"select", OperationPerformance}}
	withoutGrant := vault.Scope{Tables: []string{"orders"}, Operations: []string{"select"}}

	cases := []struct {
		name    string
		scope   vault.Scope
		engine  Engine
		sql     string
		allowed bool
	}{
		{"view with grant", withGrant, EngineMySQL, "SELECT digest_text FROM performance_schema.events_statements_summary_by_digest", true},
		{"view without grant", withoutGrant, EngineMySQL, "SELECT digest_text FROM performance_schema.events_statements_summary_by_digest", false},
		{"view joined with granted table", withGrant, EngineMySQL, "SELECT o.id FROM orders o JOIN sys.statement_analysis s ON o.id = s.rows_sent", true},
		{"view joined with ungranted table", withGrant, EngineMySQL, "SELECT s.db FROM salaries e JOIN sys.statement_analysis s ON e.id = s.rows_sent", false},
		{"system table off the allowlist", withGrant, EngineMySQL, "SELECT * FROM performance_schema.threads", false},
		{"mysql user table", withGrant, EngineMySQL, "SELECT * FROM mysql.user", false},
		{"information_schema", withGrant, EngineMySQL, "SELECT * FROM information_schema.tables", false},
		{"bare name of a view is not the view", withGrant, EngineMySQL, "SELECT * FROM statement_analysis", false},
		{"other db qualifier needs its own grant", withGrant, EngineMySQL, "SELECT * FROM otherdb.orders", false},
		{"postgres other schema needs its own grant", withGrant, EnginePostgres, "SELECT * FROM otherschema.orders", false},
		{"postgres bare view with grant", withGrant, EnginePostgres, "SELECT * FROM pg_stat_statements", true},
		{"postgres bare view without grant", withoutGrant, EnginePostgres, "SELECT * FROM pg_stat_statements", false},
		{"postgres pg_catalog view with grant", withGrant, EnginePostgres, "SELECT * FROM pg_catalog.pg_locks", true},
		{"postgres pg_catalog table off the allowlist", withGrant, EnginePostgres, "SELECT * FROM pg_catalog.pg_class", false},
		{"mysql explain of a view", vault.Scope{Operations: []string{"explain", OperationPerformance}}, EngineMySQL, "EXPLAIN SELECT digest_text FROM performance_schema.events_statements_summary_by_digest", true},
		{"postgres explain of a view", vault.Scope{Operations: []string{"explain", OperationPerformance}}, EnginePostgres, "EXPLAIN SELECT * FROM pg_stat_statements", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stmt, err := ClassifyQuery(c.engine, c.sql)
			if err != nil {
				t.Fatalf("ClassifyQuery: %v", err)
			}
			err = AllowStatement(c.scope, stmt)
			if c.allowed && err != nil {
				t.Errorf("AllowStatement: %v", err)
			}
			if !c.allowed {
				var violation *ErrScopeViolation
				if !errors.As(err, &violation) {
					t.Errorf("AllowStatement error = %v, want ErrScopeViolation", err)
				}
			}
		})
	}
}
