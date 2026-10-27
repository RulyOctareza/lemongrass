// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agent

import (
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/vault"
)

const testRootSecret = "correct horse battery staple"

// unreachableConnString points at a port nothing listens on, on loopback -- connecting to it
// fails fast with "connection refused" rather than hanging, so tests can drive a query all the
// way to the vault's execution step without a real database or any real network access.
const unreachableConnString = "mysql://root:secret@127.0.0.1:1/appdb"

func fullScope() vault.Scope {
	return vault.Scope{Tables: []string{"employees"}, Operations: []string{"select"}}
}

// startTestVault brings up a real vault daemon over its own unix socket.
func startTestVault(t *testing.T) *gatekeeper.Client {
	t.Helper()
	svc, err := gatekeeper.NewBackend(t.TempDir())
	if err != nil {
		t.Fatalf("vault.NewService: %v", err)
	}
	sockPath := filepath.Join(t.TempDir(), "vault.sock")
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	limiter := vault.NewFailureLimiter(1000, time.Minute, time.Hour)
	go gatekeeper.Serve(svc, l, limiter)
	t.Cleanup(func() { l.Close() })
	return &gatekeeper.Client{SocketPath: sockPath}
}

// wantsExecution asserts err is the "reached the database" failure the vault reports for an
// unreachable connection -- proof the query got all the way through the agent and vault's
// decrypt/classify/scope checks before failing, as opposed to failing at one of those earlier.
func wantsExecution(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("Query against an unreachable database returned nil error")
	}
	if !strings.Contains(err.Error(), "dbgate: executing query") {
		t.Errorf("Query error = %q, want it to fail at execution, not earlier", err.Error())
	}
}

func TestServiceRegisterThenQuery(t *testing.T) {
	vaultClient := startTestVault(t)
	if err := vaultClient.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := vaultClient.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	svc := NewService(vaultClient)
	shortID, err := svc.RegisterChannel(c.ID)
	if err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if len(shortID) != shortIDLength {
		t.Errorf("RegisterChannel returned id of length %d, want %d", len(shortID), shortIDLength)
	}

	_, err = svc.Query(shortID, "SELECT id FROM employees")
	wantsExecution(t, err)
}

// wantsTransaction asserts err is the "reached the database" failure the vault reports when it
// gets as far as opening a transaction against an unreachable connection.
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

func TestServiceRegisterThenExecute(t *testing.T) {
	vaultClient := startTestVault(t)
	if err := vaultClient.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := vaultClient.CreateChannel(testRootSecret, "test-channel", "app-backend", writeScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	svc := NewService(vaultClient)
	shortID, err := svc.RegisterChannel(c.ID)
	if err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}

	_, err = svc.Execute(shortID, "DELETE FROM employees WHERE id = 1", true, "tester")
	wantsTransaction(t, err)
}

func TestServiceExecuteUnknownShortIDReturnsErrNoSuchChannel(t *testing.T) {
	svc := NewService(startTestVault(t))
	if _, err := svc.Execute("BOGUS1", "DELETE FROM employees WHERE id = 1", true, "tester"); !errors.Is(err, ErrNoSuchChannel) {
		t.Errorf("Execute with an unregistered short id: got %v, want ErrNoSuchChannel", err)
	}
}

func TestServiceRegisterChannelRejectsUnknownRealID(t *testing.T) {
	vaultClient := startTestVault(t)
	svc := NewService(vaultClient)
	if _, err := svc.RegisterChannel("nonexistent-real-id"); err == nil {
		t.Error("RegisterChannel with an unknown real channel id returned nil error")
	}
}

func TestServiceQueryUnknownShortIDReturnsErrNoSuchChannel(t *testing.T) {
	svc := NewService(startTestVault(t))
	if _, err := svc.Query("BOGUS1", "SELECT id FROM employees"); !errors.Is(err, ErrNoSuchChannel) {
		t.Errorf("Query with an unregistered short id: got %v, want ErrNoSuchChannel", err)
	}
}

func TestServiceForgetThenQueryReturnsErrNoSuchChannel(t *testing.T) {
	vaultClient := startTestVault(t)
	if err := vaultClient.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := vaultClient.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	svc := NewService(vaultClient)
	shortID, err := svc.RegisterChannel(c.ID)
	if err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}

	svc.Forget(shortID)
	if _, err := svc.Query(shortID, "SELECT id FROM employees"); !errors.Is(err, ErrNoSuchChannel) {
		t.Errorf("Query after Forget: got %v, want ErrNoSuchChannel", err)
	}
}

func TestServiceForgetUnknownShortIDIsNoop(t *testing.T) {
	svc := NewService(startTestVault(t))
	svc.Forget("BOGUS1") // must not panic on a mapping that was never there
}

func TestServiceQueryOnOutOfScopeTableFails(t *testing.T) {
	vaultClient := startTestVault(t)
	if err := vaultClient.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := vaultClient.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	svc := NewService(vaultClient)
	shortID, err := svc.RegisterChannel(c.ID)
	if err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}

	if _, err := svc.Query(shortID, "SELECT id FROM salaries"); err == nil {
		t.Error("Query on an out-of-scope table returned nil error")
	}
}
