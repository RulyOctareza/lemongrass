// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"database/sql"
	"fmt"
	"net"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/faizalv/lemongrass/internal/vault"
)

// startTestMySQLProxy starts svc.ServeMySQLProxy for id on a loopback listener and returns its
// address, tearing the listener down when the test ends.
func startTestMySQLProxy(t *testing.T, svc *testSvc, id vault.ChannelID) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go svc.ServeMySQLProxy(ln, id)
	return ln.Addr().String()
}

// mysqlProxyDSN builds a go-sql-driver/mysql DSN for addr with password as the login password, with allowCleartextPasswords set since the proxy only speaks mysql_clear_password.
func mysqlProxyDSN(addr, password string) string {
	return fmt.Sprintf("anyuser:%s@tcp(%s)/?allowCleartextPasswords=true", password, addr)
}

func TestMySQLProxyAcceptsCorrectPassphrase(t *testing.T) {
	svc := openTestService(t)
	if err := svc.SetPassphrase(testRootSecret); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := svc.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	addr := startTestMySQLProxy(t, svc, c.ID)

	db, err := sql.Open("mysql", mysqlProxyDSN(addr, testRootSecret))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping with the correct passphrase: %v", err)
	}
}

func TestMySQLProxyRejectsWrongPassphrase(t *testing.T) {
	svc := openTestService(t)
	if err := svc.SetPassphrase(testRootSecret); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := svc.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	addr := startTestMySQLProxy(t, svc, c.ID)

	db, err := sql.Open("mysql", mysqlProxyDSN(addr, "wrong passphrase"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err == nil {
		t.Fatal("Ping with the wrong passphrase returned nil error")
	}
}

func TestMySQLProxyRejectsExpiredChannel(t *testing.T) {
	svc := openTestService(t)
	if err := svc.SetPassphrase(testRootSecret); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := svc.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), -1*time.Second)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	addr := startTestMySQLProxy(t, svc, c.ID)

	db, err := sql.Open("mysql", mysqlProxyDSN(addr, testRootSecret))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err == nil {
		t.Fatal("Ping against an already-expired channel returned nil error")
	}
}

func TestMySQLProxyQueryNotBuiltYet(t *testing.T) {
	svc := openTestService(t)
	if err := svc.SetPassphrase(testRootSecret); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := svc.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	addr := startTestMySQLProxy(t, svc, c.ID)

	db, err := sql.Open("mysql", mysqlProxyDSN(addr, testRootSecret))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	if _, err := db.Query("SELECT 1"); err == nil {
		t.Error("Query before Phase 4 wires the query path returned nil error")
	}
}
