// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

const editedConnString = "mysql://root:rotated@127.0.0.1:2/appdb"

// wrappedCopy returns the connection string held in channel c's own wrapped copy, reactivating the channel to read it without touching the base credential.
func wrappedCopy(t *testing.T, svc *vault.Service, c vault.Channel) string {
	t.Helper()
	if _, err := svc.Activate(testRootSecret, c.ID, 5*time.Minute); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	_, plain, err := svc.OpenChannel(c.ID)
	if err != nil {
		t.Fatalf("reading wrapped copy of %s: %v", c.ID, err)
	}
	return string(plain)
}

func (g *Gate) cached(id vault.ChannelID) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.conns[id]
	return ok
}

func TestUpdateConnectionRewrapsActiveAndExpiredChannelsAndDropsCachedDB(t *testing.T) {
	svc, g := openTestGate(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatal(err)
	}
	active, err := svc.CreateChannel(testRootSecret, "active", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	inactive, err := svc.CreateChannel(testRootSecret, "inactive", "app-backend", fullScope(), -1*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	_, err = g.Query(active.ID, "SELECT id FROM employees")
	wantsExecution(t, err)
	if !g.cached(active.ID) {
		t.Fatal("expected the baseline query to cache a database handle")
	}

	if err := g.UpdateConnection(testRootSecret, "app-backend", editedConnString); err != nil {
		t.Fatalf("UpdateConnection: %v", err)
	}

	if g.cached(active.ID) {
		t.Error("the active channel still holds a database handle opened with the old credential")
	}
	if got := wrappedCopy(t, svc, active); got != editedConnString {
		t.Errorf("active channel copy = %q, want the edited string", got)
	}
	if got := wrappedCopy(t, svc, inactive); got != editedConnString {
		t.Errorf("inactive channel copy = %q, want the edited string", got)
	}
	if got, err := svc.GetConnection(testRootSecret, "app-backend"); err != nil || got != editedConnString {
		t.Errorf("stored connection = %q, %v, want the edited string", got, err)
	}

	_, err = g.Query(active.ID, "SELECT id FROM employees")
	wantsExecution(t, err)
	if _, err := svc.Activate(testRootSecret, inactive.ID, 5*time.Minute); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	_, err = g.Query(inactive.ID, "SELECT id FROM employees")
	wantsExecution(t, err)

	if c, err := svc.ChannelScope(active.ID); err != nil || c.Port != active.Port || c.Name != "active" {
		t.Errorf("channel metadata = %+v, %v, want it unchanged", c, err)
	}
}

func TestUpdateConnectionWrongPassphraseChangesNothing(t *testing.T) {
	svc, g := openTestGate(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatal(err)
	}
	c, err := svc.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if err := g.UpdateConnection("wrong passphrase", "app-backend", editedConnString); err == nil {
		t.Fatal("expected an error for a wrong passphrase")
	}
	if got, _ := svc.GetConnection(testRootSecret, "app-backend"); got != unreachableConnString {
		t.Errorf("stored connection = %q, want it unchanged", got)
	}
	if got := wrappedCopy(t, svc, c); got != unreachableConnString {
		t.Errorf("channel copy = %q, want it unchanged", got)
	}
}

func TestUpdateConnectionUnknownName(t *testing.T) {
	svc, g := openTestGate(t)
	if err := g.UpdateConnection(testRootSecret, "missing", editedConnString); err == nil {
		t.Error("expected an error updating a connection that was never stored")
	}
	if names, _ := svc.ListConnections(); len(names) != 0 {
		t.Errorf("connections = %v, want none created by a failed update", names)
	}
}

func TestUpdateConnectionRefusesEngineChange(t *testing.T) {
	svc, g := openTestGate(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatal(err)
	}
	err := g.UpdateConnection(testRootSecret, "app-backend", "postgres://root:secret@127.0.0.1:1/appdb")
	if err == nil || !strings.Contains(err.Error(), "engine cannot change") {
		t.Errorf("err = %v, want an engine-change refusal", err)
	}
	if got, _ := svc.GetConnection(testRootSecret, "app-backend"); got != unreachableConnString {
		t.Errorf("stored connection = %q, want it unchanged", got)
	}

	if err := g.UpdateConnection(testRootSecret, "app-backend", "mariadb://root:secret@127.0.0.1:2/appdb"); err != nil {
		t.Errorf("mysql to mariadb is the same engine, got %v", err)
	}
	if err := g.UpdateConnection(testRootSecret, "app-backend", "no-scheme"); err == nil {
		t.Error("expected an error for a connection string with no scheme")
	}
}

func TestUpdateConnectionLeavesOtherConnectionsChannelsAlone(t *testing.T) {
	svc, g := openTestGate(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatal(err)
	}
	if err := svc.PutCredential(testRootSecret, "other", []byte(unreachableConnString)); err != nil {
		t.Fatal(err)
	}
	c, err := svc.CreateChannel(testRootSecret, "on-other", "other", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.UpdateConnection(testRootSecret, "app-backend", editedConnString); err != nil {
		t.Fatal(err)
	}
	if got := wrappedCopy(t, svc, c); got != unreachableConnString {
		t.Errorf("channel on another connection = %q, want it untouched", got)
	}
}
