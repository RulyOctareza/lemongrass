// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

// Package ratelimit locks out further attempts after too many failures within a window.
package ratelimit

import (
	"fmt"
	"sync"
	"time"
)

// FailureLimiter counts failures in a sliding window and, past the threshold, refuses every attempt until the lockout ends.
type FailureLimiter struct {
	maxFailures int
	window      time.Duration
	lockout     time.Duration
	lockedErr   error

	mu          sync.Mutex
	failures    []time.Time
	lockedUntil time.Time
}

// New returns a limiter whose lockout error wraps lockedErr, so each caller keeps its own package prefix.
func New(maxFailures int, window, lockout time.Duration, lockedErr error) *FailureLimiter {
	return &FailureLimiter{maxFailures: maxFailures, window: window, lockout: lockout, lockedErr: lockedErr}
}

func (f *FailureLimiter) Check() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if remaining := time.Until(f.lockedUntil); remaining > 0 {
		return fmt.Errorf("%w, try again in %s", f.lockedErr, waitText(remaining))
	}
	return nil
}

func waitText(d time.Duration) string {
	if d >= time.Minute {
		minutes := int((d + time.Minute - 1) / time.Minute)
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	seconds := int((d + time.Second - 1) / time.Second)
	if seconds == 1 {
		return "1 second"
	}
	return fmt.Sprintf("%d seconds", seconds)
}

func (f *FailureLimiter) RecordFailure() {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-f.window)
	kept := f.failures[:0]
	for _, t := range f.failures {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	f.failures = kept

	if len(f.failures) >= f.maxFailures {
		f.lockedUntil = now.Add(f.lockout)
		f.failures = nil
	}
}

func (f *FailureLimiter) RecordSuccess() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures = nil
}
