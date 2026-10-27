// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

const testRootSecret = "correct horse battery staple"

// unreachableConnString points at a port nothing listens on, on loopback, so connecting to it
// fails fast with "connection refused" and tests can drive Query to the execution step without a
// real database or any real network access.
const unreachableConnString = "mysql://root:secret@127.0.0.1:1/appdb"

// testSvc pairs a vault with the Gate attached to it, the way the daemon does.
type testSvc struct {
	*vault.Service
	*Gate
}

func openTestService(t *testing.T) *testSvc {
	t.Helper()
	svc, g := openTestGate(t)
	return &testSvc{Service: svc, Gate: g}
}

func openTestGate(t *testing.T) (*vault.Service, *Gate) {
	t.Helper()
	svc, err := vault.NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, New(svc)
}

func fullScope() vault.Scope {
	return vault.Scope{Tables: []string{"employees"}, Operations: []string{"select"}}
}

func newTestChannel(t *testing.T, svc *vault.Service, scope vault.Scope, ttl time.Duration) vault.Channel {
	t.Helper()
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := svc.CreateChannel(testRootSecret, "test-channel", "app-backend", scope, ttl)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	return c
}

// wantsExecution asserts err is the "reached the database" failure runQuery reports for an
// unreachable connection, proof Query got all the way through decrypt/classify/scope before
// failing, as opposed to failing at one of those earlier steps.
func wantsExecution(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("Query against an unreachable database returned nil error")
	}
	if !strings.Contains(err.Error(), "dbgate: executing query") {
		t.Errorf("Query error = %q, want it to fail at execution, not earlier", err.Error())
	}
}

func TestQueryReachesExecutionAfterScopeChecksPass(t *testing.T) {
	svc, g := openTestGate(t)
	c := newTestChannel(t, svc, fullScope(), 5*time.Minute)

	_, err := g.Query(c.ID, "SELECT id FROM employees")
	wantsExecution(t, err)
}

func TestQueryDeniesOutOfScopeTable(t *testing.T) {
	svc, g := openTestGate(t)
	c := newTestChannel(t, svc, fullScope(), 5*time.Minute)

	_, err := g.Query(c.ID, "SELECT id FROM salaries")
	var violation *ErrScopeViolation
	if !errors.As(err, &violation) {
		t.Errorf("Query on an out-of-scope table: got %v, want ErrScopeViolation", err)
	}
}

func TestQueryDeniesIntrospectWithoutShowGranted(t *testing.T) {
	svc, g := openTestGate(t)
	c := newTestChannel(t, svc, fullScope(), 5*time.Minute)

	_, err := g.Query(c.ID, "SHOW TABLES")
	var violation *ErrScopeViolation
	if !errors.As(err, &violation) {
		t.Errorf("Query for SHOW without show granted: got %v, want ErrScopeViolation", err)
	}
}

func TestQueryRejectsWriteStatement(t *testing.T) {
	svc, g := openTestGate(t)
	c := newTestChannel(t, svc, fullScope(), 5*time.Minute)

	if _, err := g.Query(c.ID, "DELETE FROM employees"); err == nil {
		t.Error("Query with a write statement returned nil error")
	}
}

func TestQueryFailsAfterExpiry(t *testing.T) {
	svc, g := openTestGate(t)
	c := newTestChannel(t, svc, fullScope(), -1*time.Second)

	if _, err := g.Query(c.ID, "SELECT id FROM employees"); !errors.Is(err, vault.ErrChannelExpired) {
		t.Errorf("Query on an already-expired channel: got %v, want ErrChannelExpired", err)
	}
}

func TestQueryFailsAfterRevoke(t *testing.T) {
	svc, g := openTestGate(t)
	c := newTestChannel(t, svc, fullScope(), 5*time.Minute)
	if err := svc.Revoke(c.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	if _, err := g.Query(c.ID, "SELECT id FROM employees"); !errors.Is(err, vault.ErrNotFound) {
		t.Errorf("Query after Revoke: got %v, want ErrNotFound", err)
	}
}

func TestQueryAfterActivateReachesExecution(t *testing.T) {
	svc, g := openTestGate(t)
	c := newTestChannel(t, svc, fullScope(), -1*time.Second)
	if _, err := g.Query(c.ID, "SELECT id FROM employees"); err == nil {
		t.Fatal("expected the freshly-created channel to already be expired")
	}

	if _, err := svc.Activate(testRootSecret, c.ID, 5*time.Minute); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	_, err := g.Query(c.ID, "SELECT id FROM employees")
	wantsExecution(t, err)
}

func TestQueryFailsWhenNeverActivated(t *testing.T) {
	dir := t.TempDir()
	svc, err := vault.NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	c := newTestChannel(t, svc, fullScope(), 5*time.Minute)

	// A second Service over the same directory has the metadata and the wrapped copy but no cached key, like a vault restart.
	restarted, err := vault.NewService(dir)
	if err != nil {
		t.Fatalf("NewService after restart: %v", err)
	}
	g := New(restarted)

	if _, err := g.Query(c.ID, "SELECT id FROM employees"); !errors.Is(err, vault.ErrChannelInactive) {
		t.Errorf("Query on a channel with no cached key: got %v, want ErrChannelInactive", err)
	}
}

// wantsTransaction asserts err is the "reached the database" failure Execute reports when it
// gets as far as opening a transaction against an unreachable connection, proof Execute got all
// the way through decrypt/classify/scope before failing, as opposed to failing at one of those
// earlier steps.
func wantsTransaction(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("Execute against an unreachable database returned nil error")
	}
	if !strings.Contains(err.Error(), "dbgate: beginning transaction") {
		t.Errorf("Execute error = %q, want it to fail opening the transaction, not earlier", err.Error())
	}
}

func writeScope() vault.Scope {
	return vault.Scope{Tables: []string{"employees"}, Operations: []string{"delete"}}
}

func TestExecuteReachesTransactionAfterScopeChecksPass(t *testing.T) {
	svc, g := openTestGate(t)
	c := newTestChannel(t, svc, writeScope(), 5*time.Minute)

	_, err := g.Execute(c.ID, "DELETE FROM employees WHERE id = 1", true, "tester")
	wantsTransaction(t, err)
}

func TestExecuteDeniesOutOfScopeTable(t *testing.T) {
	svc, g := openTestGate(t)
	c := newTestChannel(t, svc, writeScope(), 5*time.Minute)

	_, err := g.Execute(c.ID, "DELETE FROM salaries WHERE id = 1", true, "tester")
	var violation *ErrScopeViolation
	if !errors.As(err, &violation) {
		t.Errorf("Execute on an out-of-scope table: got %v, want ErrScopeViolation", err)
	}
}

func TestExecuteRejectsMissingWhereBeforeReachingScope(t *testing.T) {
	svc, g := openTestGate(t)
	// No delete operation granted at all -- if scope were checked first, this would be an
	// ErrScopeViolation instead, since the WHERE-less guard runs at classification time.
	c := newTestChannel(t, svc, fullScope(), 5*time.Minute)

	_, err := g.Execute(c.ID, "DELETE FROM employees", true, "tester")
	var violation *ErrScopeViolation
	if errors.As(err, &violation) {
		t.Error("Execute on a WHERE-less DELETE returned ErrScopeViolation, want the classification-time rejection")
	}
	if err == nil {
		t.Error("Execute on a WHERE-less DELETE returned nil error")
	}
}

func TestExecuteRejectsReadStatement(t *testing.T) {
	svc, g := openTestGate(t)
	c := newTestChannel(t, svc, fullScope(), 5*time.Minute)

	if _, err := g.Execute(c.ID, "SELECT id FROM employees", true, "tester"); err == nil {
		t.Error("Execute with a read statement returned nil error")
	}
}

func TestQueryRejectsWriteStatementWithWhere(t *testing.T) {
	svc, g := openTestGate(t)
	c := newTestChannel(t, svc, writeScope(), 5*time.Minute)

	if _, err := g.Query(c.ID, "DELETE FROM employees WHERE id = 1"); err == nil {
		t.Error("Query with a valid write statement returned nil error, want the mirror rejection")
	}
}

func TestExecuteFailsAfterExpiry(t *testing.T) {
	svc, g := openTestGate(t)
	c := newTestChannel(t, svc, writeScope(), -1*time.Second)

	if _, err := g.Execute(c.ID, "DELETE FROM employees WHERE id = 1", true, "tester"); !errors.Is(err, vault.ErrChannelExpired) {
		t.Errorf("Execute on an already-expired channel: got %v, want ErrChannelExpired", err)
	}
}

func TestExecuteDryRunDoesNotAuditWithoutAStore(t *testing.T) {
	svc, g := openTestGate(t)
	c := newTestChannel(t, svc, writeScope(), 5*time.Minute)

	// g.Audit is nil, as in any Gate a test doesn't wire one for; auditWrite must be a no-op
	// rather than panicking.
	_, err := g.Execute(c.ID, "DELETE FROM employees WHERE id = 1", false, "tester")
	wantsTransaction(t, err)
}

func wantsListingTables(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("ListTables against an unreachable database returned nil error")
	}
	if !strings.Contains(err.Error(), "dbgate: listing tables") {
		t.Errorf("ListTables error = %q, want it to fail at the listing query, not earlier", err.Error())
	}
}

func TestListTablesReachesQueryAfterDecrypt(t *testing.T) {
	svc, g := openTestGate(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}

	_, err := g.ListTables(testRootSecret, "app-backend")
	wantsListingTables(t, err)
}

func TestListTablesWrongRootSecretFails(t *testing.T) {
	svc, g := openTestGate(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}

	if _, err := g.ListTables("wrong passphrase", "app-backend"); err == nil {
		t.Error("ListTables with the wrong root secret returned nil error")
	}
}
