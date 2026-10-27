// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import "testing"

func TestKeychainRoundTrip(t *testing.T) {
	const secret = "test-root-secret"
	if err := SaveRootSecret(secret); err != nil {
		t.Skipf("no OS keychain backend available in this environment: %v", err)
	}
	t.Cleanup(func() { DeleteRootSecret() })

	got, err := LoadRootSecret()
	if err != nil {
		t.Fatalf("LoadRootSecret: %v", err)
	}
	if got != secret {
		t.Errorf("LoadRootSecret returned %q, want %q", got, secret)
	}

	if err := DeleteRootSecret(); err != nil {
		t.Fatalf("DeleteRootSecret: %v", err)
	}
	if _, err := LoadRootSecret(); err == nil {
		t.Error("LoadRootSecret after Delete returned nil error, want a not-found error")
	}
}
