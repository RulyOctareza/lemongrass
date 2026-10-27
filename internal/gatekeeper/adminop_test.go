// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

func failWith(err error) func() (response, error) {
	return func() (response, error) { return response{}, err }
}

func TestAdminOpIgnoresNonAuthFailures(t *testing.T) {
	limiter := vault.NewFailureLimiter(3, time.Minute, time.Hour)
	for i := 0; i < 10; i++ {
		resp := adminOp(limiter, failWith(errors.New("dial tcp: connection refused")))
		if resp.OK {
			t.Fatal("a failing op reported OK")
		}
	}
	if err := limiter.Check(); err != nil {
		t.Fatalf("non-auth failures locked the limiter: %v", err)
	}
}

func TestAdminOpCountsWrongSecretFailures(t *testing.T) {
	limiter := vault.NewFailureLimiter(3, time.Minute, time.Hour)
	wrapped := fmt.Errorf("vault: reading credential for x: %w", vault.ErrWrongKey)
	for i := 0; i < 2; i++ {
		adminOp(limiter, failWith(wrapped))
	}
	adminOp(limiter, failWith(vault.ErrWrongPassphrase))
	if err := limiter.Check(); !errors.Is(err, vault.ErrLockedOut) {
		t.Fatalf("Check after three wrong-secret failures = %v, want ErrLockedOut", err)
	}
	resp := adminOp(limiter, failWith(errors.New("unreached")))
	if resp.OK || resp.Error == "" {
		t.Fatalf("a locked-out op = %+v, want an error response", resp)
	}
}
