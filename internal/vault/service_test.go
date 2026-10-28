// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"errors"
	"testing"
	"time"
)

const testRootSecret = "correct horse battery staple"

// unreachableConnString points at a port nothing listens on, on loopback, so it parses as a real connection string without a database behind it.
const unreachableConnString = "mysql://root:secret@127.0.0.1:1/appdb"

func openTestService(t *testing.T) *Service {
	t.Helper()
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func fullScope() Scope {
	return Scope{Tables: []string{"employees"}, Operations: []string{"select"}}
}

func newTestChannel(t *testing.T, svc *Service, ttl time.Duration) Channel {
	t.Helper()
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := svc.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), ttl)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	return c
}

func TestServiceOpenChannelReturnsDecryptedSecret(t *testing.T) {
	svc := openTestService(t)
	c := newTestChannel(t, svc, 5*time.Minute)

	got, secret, err := svc.OpenChannel(c.ID)
	if err != nil {
		t.Fatalf("OpenChannel: %v", err)
	}
	if string(secret) != unreachableConnString {
		t.Errorf("OpenChannel secret = %q, want the stored connection string", secret)
	}
	if got.DBName != "app-backend" {
		t.Errorf("OpenChannel.DBName = %q, want app-backend", got.DBName)
	}
}

func TestServiceOpenChannelFailsAfterExpiry(t *testing.T) {
	svc := openTestService(t)
	c := newTestChannel(t, svc, -1*time.Second)

	if _, _, err := svc.OpenChannel(c.ID); !errors.Is(err, ErrChannelExpired) {
		t.Errorf("OpenChannel on an expired channel: got %v, want ErrChannelExpired", err)
	}
}

func TestServiceOpenChannelFailsAfterRevoke(t *testing.T) {
	svc := openTestService(t)
	c := newTestChannel(t, svc, 5*time.Minute)
	if err := svc.Revoke(c.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	if _, _, err := svc.OpenChannel(c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("OpenChannel after Revoke: got %v, want ErrNotFound", err)
	}
}

func TestServiceOpenChannelFailsWhenNeverActivated(t *testing.T) {
	svc := openTestService(t)
	c := newTestChannel(t, svc, 5*time.Minute)

	// Simulate a vault restart: metadata and the wrapped copy persist on disk, but the in-memory key cache is gone.
	svc.mu.Lock()
	delete(svc.activeKeys, c.ID)
	svc.mu.Unlock()

	if _, _, err := svc.OpenChannel(c.ID); !errors.Is(err, ErrChannelInactive) {
		t.Errorf("OpenChannel with no cached key: got %v, want ErrChannelInactive", err)
	}
}

func TestServiceActivateReusesWrappedCopyWithoutRootCredential(t *testing.T) {
	svc := openTestService(t)
	c := newTestChannel(t, svc, -1*time.Second)
	if _, _, err := svc.OpenChannel(c.ID); err == nil {
		t.Fatal("expected the freshly-created channel to already be expired")
	}

	// Deleting the base credential proves Activate never touches it again, only the already-wrapped per-channel copy.
	if err := svc.DeleteConnection("app-backend"); err != nil {
		t.Fatalf("deleting base credential: %v", err)
	}

	if _, err := svc.Activate(testRootSecret, c.ID, 5*time.Minute); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	_, secret, err := svc.OpenChannel(c.ID)
	if err != nil {
		t.Fatalf("OpenChannel after Activate: %v", err)
	}
	if string(secret) != unreachableConnString {
		t.Errorf("OpenChannel secret = %q, want the wrapped copy's connection string", secret)
	}
}

func TestServiceInvalidateHooksRunOnRevokeAndExpiry(t *testing.T) {
	svc := openTestService(t)
	var got []ChannelID
	svc.OnInvalidate(func(id ChannelID) { got = append(got, id) })

	revoked := newTestChannel(t, svc, 5*time.Minute)
	if err := svc.Revoke(revoked.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	expired, err := svc.CreateChannel(testRootSecret, "expired-channel", "app-backend", fullScope(), -1*time.Second)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if _, _, err := svc.OpenChannel(expired.ID); !errors.Is(err, ErrChannelExpired) {
		t.Fatalf("OpenChannel on an expired channel: got %v, want ErrChannelExpired", err)
	}

	if len(got) != 2 || got[0] != revoked.ID || got[1] != expired.ID {
		t.Errorf("invalidated channels = %v, want [%s %s]", got, revoked.ID, expired.ID)
	}
}

func TestServiceCreateChannelWrongRootSecretFails(t *testing.T) {
	svc := openTestService(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte("creds")); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	if _, err := svc.CreateChannel("wrong passphrase", "test-channel", "app-backend", fullScope(), 5*time.Minute); err == nil {
		t.Error("CreateChannel with the wrong root secret returned nil error")
	}
}

func TestServiceChannelScopeReadableWithoutActivation(t *testing.T) {
	svc := openTestService(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte("creds")); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := svc.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	svc.mu.Lock()
	delete(svc.activeKeys, c.ID)
	svc.mu.Unlock()

	got, err := svc.ChannelScope(c.ID)
	if err != nil {
		t.Fatalf("ChannelScope on an inactive channel: %v", err)
	}
	if got.DBName != "app-backend" {
		t.Errorf("ChannelScope.DBName = %q, want app-backend", got.DBName)
	}
}

func TestServiceListChannels(t *testing.T) {
	svc := openTestService(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte("creds")); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}

	empty, err := svc.ListChannels()
	if err != nil {
		t.Fatalf("ListChannels on an empty vault: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListChannels on an empty vault = %v, want none", empty)
	}

	a, err := svc.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel a: %v", err)
	}
	b, err := svc.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel b: %v", err)
	}

	channels, err := svc.ListChannels()
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("len(ListChannels()) = %d, want 2", len(channels))
	}
	ids := map[ChannelID]bool{channels[0].ID: true, channels[1].ID: true}
	if !ids[a.ID] || !ids[b.ID] {
		t.Errorf("ListChannels() = %v, want to include %s and %s", channels, a.ID, b.ID)
	}

	if err := svc.Revoke(a.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	channels, err = svc.ListChannels()
	if err != nil {
		t.Fatalf("ListChannels after revoke: %v", err)
	}
	if len(channels) != 1 || channels[0].ID != b.ID {
		t.Fatalf("ListChannels after revoking a = %v, want only %s", channels, b.ID)
	}
}
