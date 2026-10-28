// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"bytes"
	"errors"
	"testing"
)

func openTestStore(t *testing.T) (*Store, []byte) {
	t.Helper()
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	key, err := DeriveKey("passphrase", []byte("0123456789abcdef"))
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	return store, key
}

func TestStorePutGetRoundTrip(t *testing.T) {
	store, key := openTestStore(t)
	want := []byte("postgres://app-backend-creds")
	if err := store.Put("app-backend", key, want); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get("app-backend", key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Get returned %q, want %q", got, want)
	}
}

func TestStoreGetWrongKeyFails(t *testing.T) {
	store, key := openTestStore(t)
	if err := store.Put("app-backend", key, []byte("secret")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	wrongKey, err := DeriveKey("wrong passphrase", []byte("0123456789abcdef"))
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	if _, err := store.Get("app-backend", wrongKey); err == nil {
		t.Error("Get with the wrong key returned nil error, want a decryption failure")
	}
}

func TestStoreGetMissingReturnsErrNotFound(t *testing.T) {
	store, key := openTestStore(t)
	if _, err := store.Get("reports-backend", key); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get on a missing name: got %v, want ErrNotFound", err)
	}
}

func TestStoreDeleteThenGetReturnsErrNotFound(t *testing.T) {
	store, key := openTestStore(t)
	if err := store.Put("app-backend", key, []byte("secret")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Delete("app-backend"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get("app-backend", key); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: got %v, want ErrNotFound", err)
	}
}

func TestStoreDeleteMissingIsNotAnError(t *testing.T) {
	store, _ := openTestStore(t)
	if err := store.Delete("never-existed"); err != nil {
		t.Errorf("Delete on a name that was never Put: %v, want nil", err)
	}
}
