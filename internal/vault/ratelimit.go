// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package vault

import (
	"errors"
	"time"

	"github.com/faizalv/lemongrass/internal/ratelimit"
)

var ErrLockedOut = errors.New("vault: too many failed attempts, locked out")

// FailureLimiter guards the operations that take a root secret over the vault's unix socket, which anything running as the same OS user can reach.
type FailureLimiter = ratelimit.FailureLimiter

func NewFailureLimiter(maxFailures int, window, lockout time.Duration) *FailureLimiter {
	return ratelimit.New(maxFailures, window, lockout, ErrLockedOut)
}

// IsAuthFailure reports whether err means the supplied root secret was wrong, the only failure a root-secret limiter should count.
func IsAuthFailure(err error) bool {
	return errors.Is(err, ErrWrongPassphrase) || errors.Is(err, ErrWrongKey)
}
