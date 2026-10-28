// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import "testing"

func TestGetConnectionReturnsStoredString(t *testing.T) {
	svc := openTestService(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetConnection(testRootSecret, "app-backend")
	if err != nil || got != unreachableConnString {
		t.Errorf("GetConnection = %q, %v, want the stored string", got, err)
	}
	if _, err := svc.GetConnection("wrong passphrase", "app-backend"); err == nil {
		t.Error("GetConnection accepted a wrong passphrase")
	}
	if _, err := svc.GetConnection(testRootSecret, "missing"); err == nil {
		t.Error("GetConnection returned nil error for an unknown name")
	}
}
