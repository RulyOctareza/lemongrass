// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"errors"
	"testing"
	"time"
)

func TestFailureLimiterLockoutStatesWaitTime(t *testing.T) {
	f := NewFailureLimiter(1, time.Minute, 5*time.Minute)
	f.RecordFailure()
	err := f.Check()
	if !errors.Is(err, ErrLockedOut) {
		t.Fatalf("Check = %v, want ErrLockedOut", err)
	}
	if got, want := err.Error(), "vault: too many failed attempts, locked out, try again in 5 minutes"; got != want {
		t.Errorf("Check error = %q, want %q", got, want)
	}
}
