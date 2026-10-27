// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package ratelimit

import (
	"errors"
	"testing"
	"time"
)

var errLocked = errors.New("test: locked out")

func TestFailureLimiterLocksOutAtThreshold(t *testing.T) {
	f := New(3, time.Minute, time.Hour, errLocked)
	for i := 0; i < 2; i++ {
		if err := f.Check(); err != nil {
			t.Fatalf("Check before threshold: %v", err)
		}
		f.RecordFailure()
	}
	if err := f.Check(); err != nil {
		t.Fatalf("Check on the failure just short of threshold: %v", err)
	}
	f.RecordFailure()
	if err := f.Check(); !errors.Is(err, errLocked) {
		t.Errorf("Check at threshold: got %v, want errLocked", err)
	}
}

func TestFailureLimiterSuccessResetsCount(t *testing.T) {
	f := New(3, time.Minute, time.Hour, errLocked)
	f.RecordFailure()
	f.RecordFailure()
	f.RecordSuccess()
	f.RecordFailure()
	if err := f.Check(); err != nil {
		t.Errorf("Check after a success reset the count: got %v, want nil", err)
	}
}

func TestFailureLimiterOldFailuresOutsideWindowDontCount(t *testing.T) {
	f := New(2, 10*time.Millisecond, time.Hour, errLocked)
	f.RecordFailure()
	time.Sleep(20 * time.Millisecond)
	f.RecordFailure()
	if err := f.Check(); err != nil {
		t.Errorf("Check with the first failure outside the window: got %v, want nil", err)
	}
}

func TestWaitText(t *testing.T) {
	cases := map[time.Duration]string{
		time.Second:                    "1 second",
		30 * time.Second:               "30 seconds",
		time.Minute:                    "1 minute",
		time.Minute + time.Second:      "2 minutes",
		4*time.Minute + 30*time.Second: "5 minutes",
	}
	for d, want := range cases {
		if got := waitText(d); got != want {
			t.Errorf("waitText(%v) = %q, want %q", d, got, want)
		}
	}
}
