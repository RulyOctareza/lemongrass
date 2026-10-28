// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"bytes"
	"testing"
	"time"
)

func TestNewChannelIDUnique(t *testing.T) {
	a, err := NewChannelID()
	if err != nil {
		t.Fatalf("NewChannelID: %v", err)
	}
	b, err := NewChannelID()
	if err != nil {
		t.Fatalf("NewChannelID: %v", err)
	}
	if a == "" || b == "" {
		t.Fatal("NewChannelID returned an empty id")
	}
	if a == b {
		t.Fatalf("two calls to NewChannelID returned the same id: %q", a)
	}
}

func TestDeriveKeyDeterministicAndSized(t *testing.T) {
	salt, err := NewSalt()
	if err != nil {
		t.Fatalf("NewSalt: %v", err)
	}
	k1, err := DeriveKey("correct horse battery staple", salt)
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	k2, err := DeriveKey("correct horse battery staple", salt)
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	if !bytes.Equal(k1, k2) {
		t.Fatal("DeriveKey returned different keys for the same passphrase and salt")
	}
	if len(k1) != 32 {
		t.Fatalf("len(DeriveKey(...)) = %d, want 32", len(k1))
	}
}

func TestDeriveKeyDifferentSaltsDiverge(t *testing.T) {
	saltA, err := NewSalt()
	if err != nil {
		t.Fatalf("NewSalt: %v", err)
	}
	saltB, err := NewSalt()
	if err != nil {
		t.Fatalf("NewSalt: %v", err)
	}
	kA, err := DeriveKey("same passphrase", saltA)
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	kB, err := DeriveKey("same passphrase", saltB)
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	if bytes.Equal(kA, kB) {
		t.Fatal("DeriveKey produced the same key from two different salts")
	}
}

func TestDeriveKeyRejectsEmptyInputs(t *testing.T) {
	salt, _ := NewSalt()
	if _, err := DeriveKey("", salt); err != ErrEmptyPassphrase {
		t.Errorf("DeriveKey with empty passphrase: got %v, want ErrEmptyPassphrase", err)
	}
	if _, err := DeriveKey("passphrase", nil); err != ErrEmptySalt {
		t.Errorf("DeriveKey with empty salt: got %v, want ErrEmptySalt", err)
	}
}

func TestChannelExpired(t *testing.T) {
	now := time.Now()
	c := Channel{ExpiresAt: now.Add(5 * time.Minute)}
	if c.Expired(now) {
		t.Error("channel with a future ExpiresAt reported as expired")
	}
	if !c.Expired(now.Add(6 * time.Minute)) {
		t.Error("channel past its ExpiresAt reported as not expired")
	}
	if !c.Expired(c.ExpiresAt) {
		t.Error("channel at exactly its ExpiresAt reported as not expired")
	}
}
