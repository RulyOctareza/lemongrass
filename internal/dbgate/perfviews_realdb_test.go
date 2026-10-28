// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

func performanceChannel(t *testing.T, dsn string, operations ...string) (*testSvc, vault.ChannelID) {
	t.Helper()
	svc := openTestService(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(dsn)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := svc.CreateChannel(testRootSecret, "perf-channel", "app-backend", vault.Scope{Operations: operations}, 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	return svc, c.ID
}

func quoteMySQLName(name string) string {
	schema, table, _ := strings.Cut(name, ".")
	return "`" + schema + "`.`" + table + "`"
}

// TestPerfViewsAgainstRealMySQL needs LGRASS_TEST_MYSQL_DSN, a mysql:// connection string whose user can read performance_schema and sys.
func TestPerfViewsAgainstRealMySQL(t *testing.T) {
	dsn := os.Getenv("LGRASS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("LGRASS_TEST_MYSQL_DSN not set, skipping real-MySQL test")
	}

	setupDSN, err := mysqlDSN(dsn)
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}
	setup, err := sql.Open("mysql", setupDSN)
	if err != nil {
		t.Fatalf("opening setup connection: %v", err)
	}
	defer setup.Close()

	for name, restricted := range perfViews[EngineMySQL] {
		for _, col := range restricted {
			if _, err := setup.Exec("SELECT " + col + " FROM " + name + " LIMIT 0"); err != nil {
				t.Errorf("restricted column %s does not exist on %s: %v", col, name, err)
			}
		}
	}

	svc, id := performanceChannel(t, dsn, "select", OperationPerformance)
	for _, name := range PerfViewNames(EngineMySQL) {
		if _, err := svc.Query(id, "SELECT COUNT(*) FROM "+quoteMySQLName(name)); err != nil {
			t.Errorf("allowlisted view %s: %v", name, err)
		}
	}

	if _, err := svc.Query(id, "SELECT digest_text, count_star FROM "+digestTable+" ORDER BY sum_timer_wait DESC LIMIT 5"); err != nil {
		t.Errorf("digest query with named columns: %v", err)
	}
	for _, denied := range []string{
		"SELECT * FROM " + digestTable,
		"SELECT query_sample_text FROM " + digestTable,
	} {
		if _, err := svc.Query(id, denied); err == nil {
			t.Errorf("Query(%q): expected rejection", denied)
		}
	}
	if _, err := svc.Query(id, "SELECT * FROM performance_schema.threads"); err == nil {
		t.Error("a system table off the allowlist should be denied even with the grant")
	}
	if _, err := svc.Query(id, "SELECT user FROM mysql.user"); err == nil {
		t.Error("mysql.user should be denied even with the grant")
	}

	plain, plainID := performanceChannel(t, dsn, "select")
	if _, err := plain.Query(plainID, "SELECT count_star FROM "+digestTable); err == nil {
		t.Error("a channel without the performance operation should be denied")
	}
}

// TestPerfViewsAgainstRealPostgres needs LGRASS_TEST_POSTGRES_DSN, a postgres:// connection string for a server started with pg_stat_statements preloaded.
func TestPerfViewsAgainstRealPostgres(t *testing.T) {
	dsn := os.Getenv("LGRASS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("LGRASS_TEST_POSTGRES_DSN not set, skipping real-Postgres test")
	}

	setup, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("opening setup connection: %v", err)
	}
	defer setup.Close()
	if _, err := setup.Exec("CREATE EXTENSION IF NOT EXISTS pg_stat_statements"); err != nil {
		t.Fatalf("creating pg_stat_statements: %v", err)
	}

	svc, id := performanceChannel(t, dsn, "select", "explain", OperationPerformance)
	for _, name := range PerfViewNames(EnginePostgres) {
		if _, err := svc.Query(id, "SELECT COUNT(*) FROM "+name); err != nil {
			t.Errorf("allowlisted view %s: %v", name, err)
		}
	}
	if _, err := svc.Query(id, "SELECT COUNT(*) FROM pg_catalog.pg_locks"); err != nil {
		t.Errorf("pg_catalog-qualified view: %v", err)
	}
	if _, err := svc.Query(id, "SELECT relname FROM pg_catalog.pg_class"); err == nil {
		t.Error("a pg_catalog table off the allowlist should be denied even with the grant")
	}

	plain, plainID := performanceChannel(t, dsn, "select")
	if _, err := plain.Query(plainID, "SELECT COUNT(*) FROM pg_stat_statements"); err == nil {
		t.Error("a channel without the performance operation should be denied")
	}
}
