// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/channelaudit"
	"github.com/faizalv/lemongrass/internal/vault"
)

// TestServiceQueryAgainstRealMySQL exercises the whole Phase 3 path -- classify, scope,
// connection caching, execution, row normalization -- against a real server
// instead of the unreachable-port trick the rest of this package's tests use. It needs
// LGRASS_TEST_MYSQL_DSN set to a mysql://user:pass@host:port/db connection string reachable
// from this machine (a local docker mysql works); it's skipped otherwise.
func TestServiceQueryAgainstRealMySQL(t *testing.T) {
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

	if _, err := setup.Exec(`DROP TABLE IF EXISTS lg_test_employees`); err != nil {
		t.Fatalf("dropping lg_test_employees: %v", err)
	}
	if _, err := setup.Exec(`CREATE TABLE lg_test_employees (id INT, name VARCHAR(64), manager VARCHAR(64) NULL)`); err != nil {
		t.Fatalf("creating lg_test_employees: %v", err)
	}
	t.Cleanup(func() { setup.Exec(`DROP TABLE IF EXISTS lg_test_employees`) })
	if _, err := setup.Exec(`INSERT INTO lg_test_employees (id, name, manager) VALUES (1, 'Alice', NULL), (2, 'Bob', 'Alice')`); err != nil {
		t.Fatalf("seeding lg_test_employees: %v", err)
	}

	svc := openTestService(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(dsn)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	scope := vault.Scope{Tables: []string{"lg_test_employees"}, Operations: []string{"select", "show"}}
	c, err := svc.CreateChannel(testRootSecret, "test-channel", "app-backend", scope, 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	result, err := svc.Query(c.ID, "SELECT id, name, manager FROM lg_test_employees ORDER BY id")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	wantColumns := []string{"id", "name", "manager"}
	if len(result.Columns) != len(wantColumns) {
		t.Fatalf("Columns = %v, want %v", result.Columns, wantColumns)
	}
	for i, c := range wantColumns {
		if result.Columns[i] != c {
			t.Errorf("Columns[%d] = %q, want %q", i, result.Columns[i], c)
		}
	}
	if len(result.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(result.Rows))
	}
	if result.Rows[0][1] != "Alice" || result.Rows[0][2] != nil {
		t.Errorf("Rows[0] = %v, want [.. Alice <nil>]", result.Rows[0])
	}
	if result.Rows[1][1] != "Bob" || result.Rows[1][2] != "Alice" {
		t.Errorf("Rows[1] = %v, want [.. Bob Alice]", result.Rows[1])
	}

	// Calling Query again should reuse the cached *sql.DB rather than opening a second connection.
	if _, err := svc.Query(c.ID, "SELECT id FROM lg_test_employees"); err != nil {
		t.Fatalf("second Query: %v", err)
	}
	svc.mu.Lock()
	numConns := len(svc.conns)
	svc.mu.Unlock()
	if numConns != 1 {
		t.Errorf("len(dbConns) = %d, want 1 cached connection", numConns)
	}

	showResult, err := svc.Query(c.ID, "SHOW TABLES")
	if err != nil {
		t.Fatalf("SHOW TABLES: %v", err)
	}
	if len(showResult.Rows) != 1 || showResult.Rows[0][0] != "lg_test_employees" {
		t.Errorf("SHOW TABLES rows = %v, want just [[lg_test_employees]] since the channel's scope grants only that table", showResult.Rows)
	}

	tables, err := svc.ListTables(testRootSecret, "app-backend")
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	found := false
	for _, name := range tables {
		if name == "lg_test_employees" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ListTables = %v, want it to include lg_test_employees", tables)
	}

	if err := svc.Revoke(c.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	svc.mu.Lock()
	numConns = len(svc.conns)
	svc.mu.Unlock()
	if numConns != 0 {
		t.Errorf("len(dbConns) after Revoke = %d, want 0", numConns)
	}
}

// TestServiceExecuteAgainstRealMySQL exercises the whole write path -- classify, the WHERE-less
// guard, scope, the transaction itself, and the audit row -- against a real server. It needs
// LGRASS_TEST_MYSQL_DSN set the same way TestServiceQueryAgainstRealMySQL does; skipped otherwise.
func TestServiceExecuteAgainstRealMySQL(t *testing.T) {
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

	if _, err := setup.Exec(`DROP TABLE IF EXISTS lg_test_write_employees`); err != nil {
		t.Fatalf("dropping lg_test_write_employees: %v", err)
	}
	if _, err := setup.Exec(`CREATE TABLE lg_test_write_employees (id INT, name VARCHAR(64))`); err != nil {
		t.Fatalf("creating lg_test_write_employees: %v", err)
	}
	t.Cleanup(func() { setup.Exec(`DROP TABLE IF EXISTS lg_test_write_employees`) })
	if _, err := setup.Exec(`INSERT INTO lg_test_write_employees (id, name) VALUES (1, 'Alice')`); err != nil {
		t.Fatalf("seeding lg_test_write_employees: %v", err)
	}

	svc, g := openTestGate(t)
	auditPath := filepath.Join(t.TempDir(), "audit.db")
	auditStore, err := channelaudit.Open(auditPath)
	if err != nil {
		t.Fatalf("channelaudit.Open: %v", err)
	}
	t.Cleanup(func() { auditStore.Close() })
	g.Audit = auditStore

	if err := svc.PutCredential(testRootSecret, "app-backend-write", []byte(dsn)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	scope := vault.Scope{Tables: []string{"lg_test_write_employees"}, Operations: []string{"update", "delete"}}
	c, err := svc.CreateChannel(testRootSecret, "test-channel", "app-backend-write", scope, 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	// A WHERE-less DELETE is rejected before it ever touches the database.
	if _, err := g.Execute(c.ID, "DELETE FROM lg_test_write_employees", true, "tester"); err == nil {
		t.Error("Execute with a WHERE-less DELETE returned nil error")
	}

	// A dry run reports the row it would have changed but leaves it untouched.
	dryResult, err := g.Execute(c.ID, "UPDATE lg_test_write_employees SET name = 'Zara' WHERE id = 1", false, "tester")
	if err != nil {
		t.Fatalf("dry-run Execute: %v", err)
	}
	if !dryResult.DryRun || dryResult.AffectedRows != 1 {
		t.Errorf("dry-run result = %+v, want {AffectedRows:1 DryRun:true}", dryResult)
	}
	var name string
	if err := setup.QueryRow(`SELECT name FROM lg_test_write_employees WHERE id = 1`).Scan(&name); err != nil {
		t.Fatalf("reading back row after dry run: %v", err)
	}
	if name != "Alice" {
		t.Errorf("name after dry run = %q, want it unchanged at Alice", name)
	}

	// A commit actually changes the row.
	commitResult, err := g.Execute(c.ID, "UPDATE lg_test_write_employees SET name = 'Zara' WHERE id = 1", true, "tester")
	if err != nil {
		t.Fatalf("commit Execute: %v", err)
	}
	if commitResult.DryRun || commitResult.AffectedRows != 1 {
		t.Errorf("commit result = %+v, want {AffectedRows:1 DryRun:false}", commitResult)
	}
	if err := setup.QueryRow(`SELECT name FROM lg_test_write_employees WHERE id = 1`).Scan(&name); err != nil {
		t.Fatalf("reading back row after commit: %v", err)
	}
	if name != "Zara" {
		t.Errorf("name after commit = %q, want Zara", name)
	}

	// The WHERE-less rejection never opened a transaction, so it never audited; the dry run and
	// the commit both did.
	auditDB, err := sql.Open("sqlite", auditPath)
	if err != nil {
		t.Fatalf("opening audit db: %v", err)
	}
	defer auditDB.Close()
	var count int
	if err := auditDB.QueryRow(`SELECT COUNT(*) FROM lg_channel_audit WHERE channel_id = ?`, string(c.ID)).Scan(&count); err != nil {
		t.Fatalf("counting audit rows: %v", err)
	}
	if count != 2 {
		t.Errorf("audit row count = %d, want 2 (dry run and commit, not the WHERE-less rejection)", count)
	}
}
